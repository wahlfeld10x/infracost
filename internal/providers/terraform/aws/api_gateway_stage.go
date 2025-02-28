package aws

import (
	"github.com/infracost/infracost/internal/resources/aws"
	"github.com/infracost/infracost/internal/schema"
)

func getAPIGatewayStageRegistryItem() *schema.RegistryItem {
	return &schema.RegistryItem{
		Name:  "aws_api_gateway_stage",
		RFunc: NewAPIGatewayStage,
	}
}
func NewAPIGatewayStage(d *schema.ResourceData, u *schema.UsageData) *schema.Resource {
	r := &aws.APIGatewayStage{Address: strPtr(d.Address), Region: strPtr(d.Get("region").String())}
	if !d.IsEmpty("cache_cluster_size") {
		r.CacheClusterSize = floatPtr(d.Get("cache_cluster_size").Float())
	}
	r.PopulateUsage(u)
	return r.BuildResource()
}
