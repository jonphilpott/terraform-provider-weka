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

func resourceFilesystemGroup() *schema.Resource {
	return &schema.Resource{
		ReadContext:   resourceFileystemGroupRead,
		CreateContext: resourceFileystemGroupCreate,
		UpdateContext: resourceFileystemGroupUpdate,
		DeleteContext: resourceFileystemGroupDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"target_ssd_retention": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
			},
			"start_demote": &schema.Schema{
				Type:     schema.TypeInt,
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

type WekaFileystemGroup struct {
	Data struct {
		Name               string `json:"name"`
		StartDemote        int    `json:"start_demote"`
		TargetSSDRetention int    `json:"target_ssd_retention"`
		UID                string `json:"uid"`
		ID                 string `json:"id"`
	} `json:"data"`
}

func extractFilesystemGroupJsonData(body []byte, d *schema.ResourceData) error {
	var kms WekaFileystemGroup

	if err := json.Unmarshal(body, &kms); err != nil {
		return err
	}

	d.SetId(kms.Data.UID)
	d.Set("start_demote", kms.Data.StartDemote)
	d.Set("target_ssd_retention", kms.Data.TargetSSDRetention)
	d.Set("name", kms.Data.Name)

	return nil
}

func resourceFileystemGroupRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*WekaClient)
	var diags diag.Diagnostics

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("fileSystemsGroups/%s", id))
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	if err := extractFilesystemGroupJsonData(body, d); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceFileystemGroupDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("fileSystemsGroups/%s", id))
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

func resourceFileystemGroupUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	updateData := make(map[string]interface{})

	if d.HasChange("name") {
		updateData["new_name"] = d.Get("name").(string)
	}

	if d.HasChange("target_ssd_retention") {
		updateData["target_ssd_retention"] = d.Get("target_ssd_retention").(int)
	}

	if d.HasChange("start_demote") {
		updateData["start_demote"] = d.Get("start_demote")
	}

	updateBody, err := json.Marshal(updateData)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL(fmt.Sprintf("fileSystemGroups/%s", d.Id()))
	req, err := http.NewRequest("PUT", url.String(), bytes.NewBuffer(updateBody))

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	extractFilesystemJsonData(body, d)

	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceFileystemGroupCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	createData := map[string]interface{}{
		"name":                 d.Get("name").(string),
		"target_ssd_retention": d.Get("target_ssd_retention").(int),
		"start_demote":         d.Get("start_demote").(int),
	}

	createBody, err := json.Marshal(createData)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("fileSystemsGroups")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	var kms WekaFileystemGroup

	if err := json.Unmarshal(body, &kms); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(kms.Data.UID)

	return diags
}
