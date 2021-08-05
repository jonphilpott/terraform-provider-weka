terraform {
	required_providers {
		weka = {
		   version = "0.1"
		   source = "github.com/jonphilpott/weka"
		}
        }
}

provider "weka" { }

data "weka_test" "test" { }

#
#resource "weka_objectstore" "obs_test1" {
#	name = "obs_test1"
#}

resource "weka_filesystem" "test1fs" {
	name = "test21"
	total_capacity_gb = 4
	encrypted = false
	auth_required = true
	allow_no_kms = true
	group_name = "default"
	tiered = false
}


resource "weka_kms" "kms_test1" {
	base_url = "https://localhost:1234/"
	master_key_name = "foo"
	token = "foobar"
}
