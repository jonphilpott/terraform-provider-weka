package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	awspolicy "github.com/hashicorp/awspolicyequivalence"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"net/http"
	"strings"
	"time"
)

func resourceS3Policy() *schema.Resource {
	return &schema.Resource{
		ReadContext:   resourceS3PolicyRead,
		CreateContext: resourceS3PolicyCreate,
		UpdateContext: resourceS3PolicyUpdate,
		DeleteContext: resourceS3PolicyDelete,
		Schema: map[string]*schema.Schema{
			"policy_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"policy_file_content": &schema.Schema{
				Type:             schema.TypeString,
				Required:         true,
				ValidateFunc:     validation.StringIsJSON,
				DiffSuppressFunc: AWSPolicyDiff,
			},
			"last_updated": &schema.Schema{
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
		},
	}
}

func AWSPolicyDiff(k, old, new string, d *schema.ResourceData) bool {
	old_blank := (strings.TrimSpace(old) == "" || strings.TrimSpace(old) == "{}")
	new_blank := (strings.TrimSpace(new) == "" || strings.TrimSpace(new) == "{}")

	if old_blank && new_blank {
		return true
	}

	equivalent, err := awspolicy.PoliciesAreEquivalent(old, new)
	if err != nil {
		return false
	}

	return equivalent
}

func resourceS3PolicyRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*WekaClient)
	var diags diag.Diagnostics

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("/s3/policies/%s", id))
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	responseDoc := make(map[string]interface{})

	if err := json.Unmarshal(body, &responseDoc); err != nil {
		return diag.FromErr(err)
	}

	var policy map[string]interface{} = responseDoc["data"].(map[string]interface{})["policy"].(map[string]interface{})

	// remarshall the policy document. urgh.
	policyDocument, _ := json.Marshal(policy["content"])

	d.Set("policy_name", policy["name"])
	d.Set("policy_file_content", string(policyDocument))
	d.SetId(policy["name"].(string))

	return diags
}

func resourceS3PolicyDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("/s3/policies/%s", id))
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

func resourceS3PolicyUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	diags := resourceS3PolicyCreate(ctx, d, m)
	d.Set("last_updated", time.Now().Format(time.RFC850))
	return diags
}

func resourceS3PolicyCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	createParams := make(map[string]interface{})

	createParams["policy_name"] = d.Get("policy_name").(string)

	// dance around json stuff
	var policyDocument map[string]interface{}
	if err := json.Unmarshal([]byte(d.Get("policy_file_content").(string)), &policyDocument); err != nil {
		return diag.FromErr(err)
	}
	createParams["policy_file_content"] = policyDocument

	createBody, err := json.Marshal(createParams)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("s3/policies")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	if err != nil {
		return diag.FromErr(err)
	}

	_, err = c.makeRequest(req)

	// if the swagger docs are to be trusted, then there's no useful
	// return from creating the policy, makeRequest will handle the
	// common error scenarios
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(d.Get("policy_name").(string))

	return diags
}
