package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"net/http"
	"time"
)

func resourceFilesystem() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages filesystems within Weka. Caveats: creating and manging a tiered file system with mulitple OBS buckets is currently not supported. A filesystems cannot be switched between tiered and non-tiered. OBS names cannot be changed. Gigabytes are defined as 1000000000 bytes",
		ReadContext:   resourceFilesystemRead,
		CreateContext: resourceFilesystemCreate,
		UpdateContext: resourceFilesystemUpdate,
		DeleteContext: resourceFilesystemDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"group_name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"total_capacity_gb": {
				Description: "total capacity in gigabytes, defined as 1000000000 bytes",
				Type:        schema.TypeInt,
				Required:    true,
			},
			"obs_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"ssd_capacity_gb": {
				Description: "SSD capacity in gigabytes, defined as 1000000000 bytes",
				Type:        schema.TypeInt,
				Optional:    true,
			},
			"encrypted": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"auth_required": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"allow_no_kms": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"tiered": {
				Type:     schema.TypeBool,
				Required: true,
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

type WekaFilesystem struct {
	Data struct {
		ID                   string `json:"id"`
		AutoMaxFiles         bool   `json:"auto_max_files"`
		UsedSsdData          int    `json:"used_ssd_data"`
		Name                 string `json:"name"`
		UID                  string `json:"uid"`
		IsRemoving           bool   `json:"is_removing"`
		GroupID              string `json:"group_id"`
		IsCreating           bool   `json:"is_creating"`
		FreeTotal            int    `json:"free_total"`
		IsEncrypted          bool   `json:"is_encrypted"`
		MetadataBudget       int    `json:"metadata_budget"`
		UsedTotalData        int    `json:"used_total_data"`
		UsedTotal            int    `json:"used_total"`
		SsdBudget            int    `json:"ssd_budget"`
		IsReady              bool   `json:"is_ready"`
		GroupName            string `json:"group_name"`
		AvailableTotal       int    `json:"available_total"`
		Status               string `json:"status"`
		UsedSsdMetadata      int    `json:"used_ssd_metadata"`
		AuthRequired         bool   `json:"auth_required"`
		AvailableSsdMetadata int    `json:"available_ssd_metadata"`
		TotalBudget          int    `json:"total_budget"`
		UsedSsd              int    `json:"used_ssd"`
		ObsBuckets           []struct {
			UID   string `json:"uid"`
			State string `json:"state"`
			ObsID string `json:"obsId"`
			Mode  string `json:"mode"`
			Name  string `json:"name"`
		} `json:"obs_buckets"`
		AvailableSsd int `json:"available_ssd"`
		FreeSsd      int `json:"free_ssd"`
	} `json:"data"`
}

const OurGb = 1000000000

func extractFilesystemJsonData(body []byte, d *schema.ResourceData) error {
	var kms WekaFilesystem

	if err := json.Unmarshal(body, &kms); err != nil {
		return err
	}

	d.SetId(kms.Data.UID)

	ssd_capacity := (kms.Data.UsedSsd + kms.Data.AvailableSsd) / OurGb
	total_capacity := (kms.Data.AvailableTotal + kms.Data.UsedTotal) / OurGb

	if len(kms.Data.ObsBuckets) > 0 {
		d.Set("ssd_capacity_gb", ssd_capacity)
		d.Set("tiered", true)

		if len(kms.Data.ObsBuckets) > 1 {
			return fmt.Errorf("Tiered filesystems with more than one OBS bucket currently not supported.")
		} else {
			d.Set("obs_name", kms.Data.ObsBuckets[0].Name)
		}
	} else {
		d.Set("tiered", false)
	}

	d.Set("total_capacity_gb", total_capacity)
	d.Set("encrypted", kms.Data.IsEncrypted)
	d.Set("auth_required", kms.Data.AuthRequired)
	d.Set("encrypted", kms.Data.IsEncrypted)
	d.Set("group_name", kms.Data.GroupName)

	return nil
}

func resourceFilesystemRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*WekaClient)
	var diags diag.Diagnostics

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("fileSystems/%s", id))
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	if err := extractFilesystemJsonData(body, d); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceFilesystemDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("fileSystems/%s", id))
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

func resourceFilesystemUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	updateData := make(map[string]interface{})

	if d.HasChange("name") {
		updateData["new_name"] = d.Get("name").(string)
	}

	if d.HasChange("tiered") {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "cannot switch type of filesystem from tiered/non-tiered",
		})
		return diags
	}

	if d.HasChange("obs_name") {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Cannot currently change the OBS name",
		})
		return diags
	}

	if d.HasChange("total_capacity_gb") {
		updateData["total_capacity"] = d.Get("total_capacity_gb").(int) * OurGb

	}

	if d.HasChange("auth_required") {
		updateData["auth_required"] = d.Get("auth_required")
	}

	if d.Get("tiered").(bool) && d.HasChange("ssd_capacity_gb") {
		updateData["ssd_capacity"] = d.Get("total_capacity_gb").(int) * OurGb
	}

	updateBody, err := json.Marshal(updateData)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL(fmt.Sprintf("fileSystems/%s", d.Id()))
	req, err := http.NewRequest("PUT", url.String(), bytes.NewBuffer(updateBody))

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	extractFilesystemJsonData(body, d)

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceFilesystemCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	createData := map[string]interface{}{
		"name":           d.Get("name").(string),
		"group_name":     d.Get("group_name").(string),
		"total_capacity": d.Get("total_capacity_gb").(int) * OurGb,
		"encrypted":      d.Get("encrypted").(bool),
		"auth_required":  d.Get("auth_required").(bool),
		"allow_no_kms":   d.Get("allow_no_kms").(bool),
	}

	obs_name := d.Get("obs_name").(string)
	ssd_capacity_gb := d.Get("ssd_capacity_gb").(int)
	tiered := d.Get("tiered").(bool)

	if tiered {
		createData["obs_name"] = obs_name
		createData["ssd_capacity"] = ssd_capacity_gb * OurGb
	}

	createBody, err := json.Marshal(createData)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("fileSystems")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	var kms WekaFilesystem

	if err := json.Unmarshal(body, &kms); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(kms.Data.UID)

	return diags
}
