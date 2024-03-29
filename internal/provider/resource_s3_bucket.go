package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"net/http"
	"regexp"
	"time"
)

func resourceS3Bucket() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages S3 Buckets in Weka.",
		ReadContext:   resourceS3BucketRead,
		CreateContext: resourceS3BucketCreate,
		UpdateContext: resourceS3BucketUpdate,
		DeleteContext: resourceS3BucketDelete,
		Schema: map[string]*schema.Schema{
			"bucket_name": {
				Description: "bucket name. renaming a bucket will result in delete & recreate",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				ValidateFunc: func(val any, key string) (warns []string, errs []error) {
					v := val.(string)

					// Bucket names must be between 3 & 63 characters
					// long
					l := len(v)
					if l < 3 || l > 63 {
						errs = append(errs, fmt.Errorf("Bucket names must be between 3 & 63 characters long."))
						return
					}

					// Bucket names can only be lowercase letters,
					// numbers, dots and hyphens, and cant start or
					// end with a dot or hyphen.
					r, _ := regexp.Compile("^[a-z0-9][a-z0-9.-]+[a-z0-9]$")
					if !r.MatchString(v) {
						errs = append(errs, fmt.Errorf("Bucket names can only be a-z, 0-9, with dots or hyphens and can only start and end with a letter or number"))
						return
					}

					return
				},
			},
			"anonymous_policy_name": {
				Description: "Name of policy to apply for anonymous access. Must be one of: none, download, upload or public.",
				Type:        schema.TypeString,
				Optional:    true,
				ValidateFunc: func(val any, key string) (warns []string, errs []error) {
					v := val.(string)

					if !(v == "none" || v == "download" || v == "upload" || v == "public") {
						errs = append(errs, fmt.Errorf("%q must be one of Must be one of: none, download, upload or public - got: %s", key, v))
					}

					return
				},
				Default: "none",
			},
			"hard_quota": {
				Description: "Storage quota, for example '1MB', cannot be used when existing_path is set",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"existing_path": {
				Description: "The Weka API does not provide a mechanism to update the existing path, updating this value will delete the bucket and create a new one.",
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
			},
			"fs_uid": {
				Description: "The Weka API does not provide a mechanism to update the FS of a bucket, updating this value will delete the bucket and create a new one.",
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
			},
			"last_updated": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
		},
	}
}

type WekaS3Bucket struct {
	Data struct {
		Buckets []struct {
			Name           string `json:"name"`
			HardLimitBytes int    `json:"hard_limit_bytes"`
			Path           string `json:"path"`
			UsedBytes      int    `json:"used_bytes"`
			FileSystem     string `json:"fs"`
		} `json:"buckets"`
	} `json:"data"`
}

func resourceS3BucketRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*WekaClient)
	var diags diag.Diagnostics

	id := d.Id()
	url := c.makeRestEndpointURL("/s3/buckets")
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	var parsed WekaS3Bucket

	if err := json.Unmarshal(body, &parsed); err != nil {
		return diag.FromErr(err)
	}

	for i := 0; i < len(parsed.Data.Buckets); i++ {
		b := parsed.Data.Buckets[i]

		// if the bucket exists, no change.
		if b.Name == id {
			return diags
		}
	}

	// the bucket wasn't found in the list, so tell terraform that it
	// needs to be recreated.
	d.SetId("")
	return diags
}

func resourceS3BucketDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("/s3/buckets/%s", id))
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

// in usual weka form, there isn't a single API call to update bucket
// resources, but 3, and existing_path cannot be changed.
func resourceS3BucketUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	id := d.Id()
	c := m.(*WekaClient)

	// enable partial state since we could be making several API calls for these changes
	d.Partial(true)

	// quota change
	if d.HasChange("hard_quota") {
		updateData := map[string]interface{}{
			"hard_quota": d.Get("hard_quota").(string),
		}

		updateBody, err := json.Marshal(updateData)

		if err != nil {
			return diag.FromErr(err)
		}

		url := c.makeRestEndpointURL(fmt.Sprintf("/s3/buckets/%s/quota", id))
		req, err := http.NewRequest("PUT", url.String(), bytes.NewBuffer(updateBody))
		_, err = c.makeRequest(req)

		if err != nil {
			return diag.FromErr(err)
		}
	}

	// policy change
	if d.HasChange("anonymous_policy_name") {
		// tell me - why is it `policy` in the create call and
		// `bucket_policy` in the update?
		updateData := map[string]interface{}{
			"bucket_policy": d.Get("anonymous_policy_name").(string),
		}

		updateBody, err := json.Marshal(updateData)

		if err != nil {
			return diag.FromErr(err)
		}

		url := c.makeRestEndpointURL(fmt.Sprintf("/s3/buckets/%s/policy", id))
		req, err := http.NewRequest("PUT", url.String(), bytes.NewBuffer(updateBody))
		_, err = c.makeRequest(req)

		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.Partial(false)
	d.Set("last_updated", time.Now().Format(time.RFC850))

	return diags
}

func resourceS3BucketCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	createParams := make(map[string]interface{})

	createParams["policy"] = d.Get("anonymous_policy_name").(string)

	createParams["bucket_name"] = d.Get("bucket_name").(string)

	if d.HasChange("hard_quota") {
		createParams["hard_quota"] = d.Get("hard_quota").(string)
	}

	createParams["fs_uid"] = d.Get("fs_uid").(string)

	if d.HasChange("existing_path") {
		createParams["existing_path"] = d.Get("existing_path").(string)
	}

	createBody, err := json.Marshal(createParams)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("s3/buckets")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	if err != nil {
		return diag.FromErr(err)
	}

	_, err = c.makeRequest(req)

	// if the swagger docs are to be trusted, then there's no useful
	// return data from creating the bucket, makeRequest will handle
	// the common error scenarios
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(d.Get("bucket_name").(string))

	return diags
}
