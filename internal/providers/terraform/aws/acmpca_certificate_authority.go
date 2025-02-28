package aws

import (
	"github.com/infracost/infracost/internal/resources/aws"
	"github.com/infracost/infracost/internal/schema"
)

func getACMPCACertificateAuthorityRegistryItem() *schema.RegistryItem {
	return &schema.RegistryItem{
		Name:  "aws_acmpca_certificate_authority",
		RFunc: NewAcmpcaCertificateAuthority,
	}
}
func NewAcmpcaCertificateAuthority(d *schema.ResourceData, u *schema.UsageData) *schema.Resource {
	r := &aws.AcmpcaCertificateAuthority{Address: strPtr(d.Address), Region: strPtr(d.Get("region").String())}
	r.PopulateUsage(u)
	return r.BuildResource()
}
