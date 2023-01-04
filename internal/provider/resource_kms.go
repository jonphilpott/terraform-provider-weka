package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"net/http"
	"strconv"
	"time"
)

func resourceKMS() *schema.Resource {
	return &schema.Resource{
		Description:   "Manage KMS resource within Weka. Note: Weka API does not provide a read API for KMS configuration, as such a KMS configuration cannot be imported nor will remote changes be detected.",
		ReadContext:   resourceKMSRead,
		CreateContext: resourceKMSCreate,
		UpdateContext: resourceKMSUpdate,
		DeleteContext: resourceKMSDelete,
		Schema: map[string]*schema.Schema{
			"base_url": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"master_key_name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"token": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("WEKA_VAULT_TOKEN", nil),
				Sensitive:   true,
			},
			"server_endpoint": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"key_uid": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("WEKA_VAULT_KEY_UID", nil),
				Sensitive:   true,
			},
			"client_cert_pem": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("WEKA_VAULT_CLIENT_CERT", nil),
				Sensitive:   true,
			},
			"client_key_pem": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("WEKA_VAULT_CLIENT_KEY", nil),
				Sensitive:   true,
			},
			"ca_cert_pem": &schema.Schema{
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("WEKA_VAULT_CA_CERT", nil),
				Sensitive:   true,
			},
			"use_vault": &schema.Schema{
				Type:     schema.TypeBool,
				Required: true,
			},
			"last_updated": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

type WekaKMS struct {
	Data struct {
		Params struct {
			MasterKeyName string `json:"master_key_name"`
			BaseURL       string `json:"base_url"`
		} `json:"params"`
		KmsType string `json:"kms_type"`
	} `json:"data"`
}

// Do Nothing. Not enough information is returned in the read to make any determination.
func resourceKMSRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	return diags	
}

func resourceKMSDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	url := c.makeRestEndpointURL("kms")
	req, err := http.NewRequest("DELETE", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	if _, err := c.makeRequest(req); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("")

	return diags
}

func resourceKMSUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	d.Set("last_updated", time.Now().Format(time.RFC850))
	return resourceKMSCreate(ctx, d, m)
}

func resourceKMSCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	createParams := make(map[string]string)

	vaultFields := []string{
		"base_url", "master_key_name", "token",
	}
	kmipFields := []string{
		"server_endpoint", "key_uid", "client_cert_pem", "client_key_pem", "ca_cert_pem",
	}

	if d.Get("use_vault").(bool) {
		for _, v := range vaultFields {
			if d.Get(v).(string) == "" {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  fmt.Sprintf("Missing configuration value for %s to configure KMS for Vault", v),
				})
				return diags
			}

			createParams[v] = d.Get(v).(string)
		}
	} else {
		for _, v := range kmipFields {
			if d.Get(v).(string) == "" {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  fmt.Sprintf("Missing configuration value for %s to configure KMIP", v),
				})
				return diags
			}

			createParams[v] = d.Get(v).(string)
		}
	}

	createBody, err := json.Marshal(createParams)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("kms")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	if _, err := c.makeRequest(req); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

	return diags
}
