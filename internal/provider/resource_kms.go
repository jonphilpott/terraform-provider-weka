package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"net/http"
	"strconv"
	"time"
)

func resourceKMS() *schema.Resource {
	return &schema.Resource{
		ReadContext:   resourceKMSRead,
		CreateContext: resourceKMSCreate,
		UpdateContext: resourceKMSUpdate,
		DeleteContext: resourceKMSDelete,
		Schema: map[string]*schema.Schema{
			"base_url": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"master_key_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
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
			"kms_type": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
				Optional: true,
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

func extractKMSJsonData(body []byte, d *schema.ResourceData) error {
	var kms WekaKMS

	if err := json.Unmarshal(body, &kms); err != nil {
		return err
	}

	d.Set("master_key_name", kms.Data.Params.MasterKeyName)
	d.Set("base_url", kms.Data.Params.BaseURL)
	d.Set("kms_type", kms.Data.KmsType)

	return nil
}

func resourceKMSRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*WekaClient)
	var diags diag.Diagnostics

	url := c.makeRestEndpointURL("kms")
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	if err := extractKMSJsonData(body, d); err != nil {
		return diag.FromErr(err)
	}

	// no ID on this object, set one to keep tf happy.
	d.SetId(strconv.FormatInt(time.Now().Unix(), 10))

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

	createBody, err := json.Marshal(map[string]string{
		"base_url":        d.Get("base_url").(string),
		"master_key_name": d.Get("master_key_name").(string),
		"token":           d.Get("token").(string),
		//"server_endpoint": d.Get("server_endpoint").(string),
		//		"key_uid":         d.Get("key_uid").(string),
		//		"client_cert_pem": d.Get("client_cert_pem").(string),
		//		"client_key_pem":  d.Get("client_key_pem").(string),
		//		"ca_cert_pem":     d.Get("ca_cert_pem").(string),
	})

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("kms")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	if _, err := c.makeRequest(req); err != nil {
		return diag.FromErr(err)
	}

	resourceKMSRead(ctx, d, m)

	return diags
}
