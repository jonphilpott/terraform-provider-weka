#!/bin/sh

rm -r .terraform.lock.hcl terraform.tfstate
terraform init
terraform apply
