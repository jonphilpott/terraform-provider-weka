package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

func init() {
	// Set descriptions to support markdown syntax, this will be used in document generation
	// and the language server.
	schema.DescriptionKind = schema.StringMarkdown

	// Customize the content of descriptions when output. For example you can add defaults on
	// to the exported descriptions if present.
	// schema.SchemaDescriptionBuilder = func(s *schema.Schema) string {
	// 	desc := s.Description
	// 	if s.Default != nil {
	// 		desc += fmt.Sprintf(" Defaults to `%v`.", s.Default)
	// 	}
	// 	return strings.TrimSpace(desc)
	// }
}

func New(version string) func() *schema.Provider {
	return func() *schema.Provider {
		p := &schema.Provider{
			Schema: map[string]*schema.Schema{
				"username": &schema.Schema{
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("WEKA_USERNAME", nil),
				},
				"password": &schema.Schema{
					Type:        schema.TypeString,
					Optional:    true,
					Sensitive:   true,
					DefaultFunc: schema.EnvDefaultFunc("WEKA_PASSWORD", nil),
				},
				"org": &schema.Schema{
					Type:        schema.TypeString,
					Optional:    true,
					DefaultFunc: schema.EnvDefaultFunc("WEKA_ORG", nil),
				},
				"endpoint": &schema.Schema{
					Type:        schema.TypeString,
					Required:    true,
					DefaultFunc: schema.EnvDefaultFunc("WEKA_ENDPOINT", nil),
				},
			},
			ResourcesMap: map[string]*schema.Resource{
				"weka_kms":              resourceKMS(),
				"weka_filesystem":       resourceFilesystem(),
				"weka_filesystem_group": resourceFilesystemGroup(),
				"weka_user":             resourceUser(),
			},
			DataSourcesMap:       map[string]*schema.Resource{},
			ConfigureContextFunc: providerConfigure,
		}

		return p
	}
}

type WekaAuthResponse struct {
	Data struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	} `json:"data"`
}

type WekaClient struct {
	authResponse WekaAuthResponse
	endPoint     *url.URL
	client       *http.Client
	org          string
}

type WekaErrorResponse struct {
	Message string `json:"message"`
	Data    struct {
		Error string `json:"error"`
	} `json:"data"`
}

func (w *WekaClient) getOrg() string {
	return w.org
}

func (w *WekaClient) makeRestEndpointURL(p string) url.URL {
	newUrl := *w.endPoint
	newUrl.Path = path.Join(newUrl.Path, p)
	return newUrl
}

func addHeadersToRequest(r *http.Request, w *WekaClient) {
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", w.authResponse.Data.AccessToken))

	if r.Method == "POST" || r.Method == "PUT" {
		r.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
}

func (w *WekaClient) makeRequest(r *http.Request) ([]byte, error) {
	addHeadersToRequest(r, w)

	requestDump, err := httputil.DumpRequest(r, true)

	if err != nil {
		return nil, err
	}

	log.Printf("[DEBUG] Weka Request: %s\n", string(requestDump))

	res, err := w.client.Do(r)

	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return nil, err
	}

	log.Printf("[DEBUG] Weka Response: %s\n", body)

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Non-200 status from Weka API: %d", res.StatusCode)
	}

	// is it JSON? is it an error?
	var wer WekaErrorResponse
	if err := json.Unmarshal([]byte(body), &wer); err != nil {
		return nil, err
	}

	// response indicates an error
	if wer.Data.Error != "" {
		return nil, fmt.Errorf("Error from Weka API: %s", wer.Message)
	}

	return body, err
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	username := d.Get("username").(string)
	password := d.Get("password").(string)
	org := d.Get("org").(string)
	endpoint := d.Get("endpoint").(string)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	c := &WekaClient{}

	if (username != "") && (password != "") && (org != "") && (endpoint != "") {
		url, err := url.ParseRequestURI(endpoint)

		if err != nil {
			return nil, diag.FromErr(err)
		}

		c.endPoint = url
		c.org = org

		// attempt the auth
		authBody, err := json.Marshal(map[string]string{
			"username": username,
			"password": password,
			"org":      org,
		})

		if err != nil {
			return nil, diag.FromErr(err)
		}

		c.client = &http.Client{
			Timeout: time.Second * 10,
		}

		// form URL.
		loginUrl := c.makeRestEndpointURL("login")

		resp, err := http.Post(
			loginUrl.String(),
			"application/json; charset=utf-8",
			bytes.NewBuffer(authBody),
		)

		if err != nil {
			return nil, diag.FromErr(err)
		}

		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)

		if resp.StatusCode != http.StatusOK {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("non-200 response from Weka API path %s", loginUrl.String()),
				Detail:   string(body),
			})
			return nil, diags
		}

		var wr WekaAuthResponse
		if err := json.Unmarshal([]byte(body), &wr); err != nil {
			return nil, diag.FromErr(err)
		}

		if strings.ToLower(wr.Data.TokenType) != "bearer" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("Unknown token type from Weka API (%s) path %s", wr.Data.TokenType, loginUrl.String()),
				Detail:   string(body),
			})
			return nil, diags
		}

		c.authResponse = wr

		return c, diags
	}

	diags = append(diags, diag.Diagnostic{
		Severity: diag.Error,
		Summary:  "Unable to create Weka client.",
		Detail:   "Missing required parameters to create and authenticate to Weka.",
	})

	return nil, diags
}
