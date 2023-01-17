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

func resourceUser() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages users in Weka. Please note that the Weka API does not provide the ability to pull the information about a single user with all the required fields, because of this it is not possible to import a user resource or detect remote changes at this time.",
		ReadContext:   resourceUserRead,
		CreateContext: resourceUserCreate,
		UpdateContext: resourceUserUpdate,
		DeleteContext: resourceUserDelete,
		Schema: map[string]*schema.Schema{
			"username": {
				Type:     schema.TypeString,
				Required: true,
			},
			"password": {
				Type:      schema.TypeString,
				Required:  true,
				Sensitive: true,
			},
			"role": {
				Description: "Must be one of: ClusterAdmin, OrgAdmin, ReadOnly, Regular or S3",
				Type:        schema.TypeString,
				Required:    true,
				ValidateFunc: func(val any, key string) (warns []string, errs []error) {
					v := val.(string)

					if !(v == "ClusterAdmin" || v == "OrgAdmin" || v == "ReadOnly" || v == "Regular" || v == "S3") {
						errs = append(errs, fmt.Errorf("%q must be one of ClusterAdmin, OrgAdmin, ReadOnly, Regular or S3, got: %s", key, v))
					}

					return
				},
			},
			"posix_uid": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"posix_gid": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
			"last_updated": {
				Type:     schema.TypeInt,
				Optional: true,
				Computed: true,
			},
		},
	}
}

type WekaUser struct {
	Data struct {
		UID      string `json:"uid"`
		OrgID    int    `json:"org_id"`
		Source   string `json:"source"`
		Username string `json:"username"`
		Role     string `json:"role"`
		PosixUID int    `json:"posix_uid"`
		PosixGID int    `json:"posix_gid"`
	} `json:"data"`
}

type WekaGetUsers struct {
	Data []struct {
		UID      string `json:"uid"`
		OrgID    int    `json:"org_id"`
		Source   string `json:"source"`
		Username string `json:"username"`
		Role     string `json:"role"`
	} `json:"data"`
}

// weka doesn't provide an API to get a single user, so we have to get
// _all_ of them
func resourceUserRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	id := d.Id()
	url := c.makeRestEndpointURL("/users")
	req, err := http.NewRequest("GET", url.String(), nil)

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	var parsed WekaGetUsers

	if err := json.Unmarshal(body, &parsed); err != nil {
		return diag.FromErr(err)
	}

	for i := 0; i < len(parsed.Data); i++ {
		b := parsed.Data[i]

		
		if b.UID == id {
			// role is the only field returned in the response that we
			// can actually edit.
			d.Set("role", b.Role)
			return diags
		}
	}

	// the user wasn't found in the list, so tell terraform that it
	// needs to be recreated.
	d.SetId("")
	return diags
}

func resourceUserDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	id := d.Id()
	url := c.makeRestEndpointURL(fmt.Sprintf("users/%s", id))
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

// updateable fields are: role, posix_uid and posix_guid via PUT
// /users/$uid password can be updated via /users/password
func resourceUserUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	// changes to un-updateable fields?
	if d.HasChange("username") {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "cannot update username",
		})
		return diags
	}

	// do we need to make an /users/password API call?
	if d.HasChange("password") {
		pud := make(map[string]interface{})
		pud["username"] = d.Get("username")
		op, np := d.GetChange("password")
		pud["old_password"] = op.(string)
		pud["new_password"] = np.(string)
		pud["org"] = c.getOrg()

		url := c.makeRestEndpointURL("/users/password")
		pb, err := json.Marshal(pud)

		if err != nil {
			return diag.FromErr(err)
		}

		req, err := http.NewRequest("PUT", url.String(), bytes.NewBuffer(pb))

		if err != nil {
			return diag.FromErr(err)
		}

		_, err = c.makeRequest(req)

		if err != nil {
			return diag.FromErr(err)
		}
	}

	// API call to /users/$uid
	if d.HasChange("posix_uid") ||
		d.HasChange("posix_gid") ||
		d.HasChange("role") {
		ud := make(map[string]interface{})

		if d.HasChange("role") {
			ud["role"] = d.Get("role").(string)
		}

		if d.HasChange("posix_uid") {
			ud["posix_uid"] = d.Get("posix_uid").(int)
		}

		if d.HasChange("posix_gid") {
			ud["posix_gid"] = d.Get("posix_gid").(int)
		}

		id := d.Id()
		url := c.makeRestEndpointURL(fmt.Sprintf("users/%s", id))
		req, err := http.NewRequest("PUT", url.String(), nil)

		if err != nil {
			return diag.FromErr(err)
		}

		_, err = c.makeRequest(req)

		if err != nil {
			return diag.FromErr(err)
		}
	}

	d.Set("last_updated", time.Now().Format(time.RFC850))
	return diags
}

func resourceUserCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	c := m.(*WekaClient)

	createParams := make(map[string]interface{})

	createParams["username"] = d.Get("username").(string)
	createParams["password"] = d.Get("password").(string)
	createParams["role"] = d.Get("role").(string)

	if d.HasChange("posix_uid") {
		createParams["posix_uid"] = d.Get("posix_uid").(int)
	}

	if d.HasChange("posix_uid") {
		createParams["posix_gid"] = d.Get("posix_gid").(int)
	}

	createBody, err := json.Marshal(createParams)

	if err != nil {
		return diag.FromErr(err)
	}

	url := c.makeRestEndpointURL("users")
	req, err := http.NewRequest("POST", url.String(), bytes.NewBuffer(createBody))

	if err != nil {
		return diag.FromErr(err)
	}

	body, err := c.makeRequest(req)

	if err != nil {
		return diag.FromErr(err)
	}

	var wekauser WekaUser

	if err := json.Unmarshal(body, &wekauser); err != nil {
		return diag.FromErr(err)
	}

	d.Set("posix_uid", wekauser.Data.PosixUID)
	d.Set("posix_gid", wekauser.Data.PosixGID)

	d.SetId(wekauser.Data.UID)

	return diags
}
