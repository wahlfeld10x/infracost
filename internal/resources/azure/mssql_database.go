package azure

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"

	"github.com/infracost/infracost/internal/resources"
	"github.com/infracost/infracost/internal/schema"
)

const (
	sqlServiceName   = "SQL Database"
	sqlProductFamily = "Databases"

	sqlServerlessTier = "general purpose - serverless"
	sqlHyperscaleTier = "hyperscale"
)

// MSSQLDatabase represents an azure microsoft sql database instance.
//
// More resource information here: https://azure.microsoft.com/en-gb/products/azure-sql/database/
// Pricing information here: https://azure.microsoft.com/en-gb/pricing/details/azure-sql-database/single/
type MSSQLDatabase struct {
	Address          string
	Region           string
	SKU              string
	LicenceType      string
	Tier             string
	Family           string
	Cores            int64
	MaxSizeGB        *int64
	ReadReplicaCount *int64
	ZoneRedundant    bool

	ExtraDataStorageGB         *int64 `infracost_usage:"extra_data_storage_gb"`
	MonthlyVCoreHours          *int64 `infracost_usage:"monthly_vcore_hours"`
	LongTermRetentionStorageGB *int64 `infracost_usage:"long_term_retention_storage_gb"`
}

// PopulateUsage parses the u schema.UsageData into the MSSQLDatabase.
func (r *MSSQLDatabase) PopulateUsage(u *schema.UsageData) {
	resources.PopulateArgsWithUsage(r, u)
}

// BuildResource builds a schema.Resource from a valid MSSQLDatabase.
// This method is called after the resource is initialised by an IaC provider.
func (r *MSSQLDatabase) BuildResource() *schema.Resource {
	var costComponents []*schema.CostComponent

	sku := strings.ToLower(r.SKU)
	if sku == "basic" || strings.HasPrefix(sku, "s") || strings.HasPrefix(sku, "p") {
		costComponents = append(costComponents, r.dtuPurchaseCostComponents()...)
	} else {
		costComponents = append(costComponents, r.vCorePurchaseCostComponents()...)
	}

	return &schema.Resource{
		Name: r.Address,
		UsageSchema: []*schema.UsageItem{
			{Key: "extra_data_storage_gb", DefaultValue: 0, ValueType: schema.Int64},
			{Key: "monthly_vcore_hours", DefaultValue: 0, ValueType: schema.Int64},
			{Key: "long_term_retention_storage_gb", DefaultValue: 0, ValueType: schema.Int64},
		},
		CostComponents: costComponents,
	}
}

func (r *MSSQLDatabase) vCorePurchaseCostComponents() []*schema.CostComponent {
	var costComponents []*schema.CostComponent

	if strings.ToLower(r.Tier) == sqlServerlessTier {
		costComponents = append(costComponents, r.serverlessComputeHoursCostComponent())
	} else {
		costComponents = append(costComponents, r.provisionedComputeCostComponent())
	}

	if strings.ToLower(r.Tier) == sqlHyperscaleTier {
		component := r.readReplicaCostComponent()
		costComponents = append(costComponents, component)
	}

	if strings.ToLower(r.Tier) != sqlServerlessTier {
		if strings.ToLower(r.LicenceType) == "licenseincluded" {
			costComponents = append(costComponents, r.sqlLicenseCostComponent())
		}
	}

	costComponents = append(costComponents, r.mssqlStorageComponent())

	if strings.ToLower(r.Tier) != sqlHyperscaleTier {
		costComponents = append(costComponents, r.longTermRetentionMSSQLCostComponent())
	}

	return costComponents
}

func (r *MSSQLDatabase) provisionedComputeCostComponent() *schema.CostComponent {
	skuName := r.mssqlSkuName(r.Cores)
	productNameRegex := fmt.Sprintf("/%s - %s/", r.Tier, r.Family)
	name := fmt.Sprintf("Compute (provisioned, %s)", r.SKU)

	log.Warnf("'Multiple products found' are safe to ignore for '%s' due to limitations in the Azure API.", name)

	return &schema.CostComponent{
		Name:           name,
		Unit:           "hours",
		UnitMultiplier: decimal.NewFromInt(1),
		HourlyQuantity: decimalPtr(decimal.NewFromInt(1)),
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(r.Region),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", ValueRegex: strPtr(productNameRegex)},
				{Key: "skuName", Value: strPtr(skuName)},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	}
}

func (r *MSSQLDatabase) readReplicaCostComponent() *schema.CostComponent {
	productNameRegex := fmt.Sprintf("/%s - %s/", r.Tier, r.Family)
	skuName := r.mssqlSkuName(r.Cores)

	var replicaCount *decimal.Decimal
	if r.ReadReplicaCount != nil {
		replicaCount = decimalPtr(decimal.NewFromInt(*r.ReadReplicaCount))
	}

	return &schema.CostComponent{
		Name:           "Read replicas",
		Unit:           "hours",
		UnitMultiplier: decimal.NewFromInt(1),
		HourlyQuantity: replicaCount,
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(r.Region),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", ValueRegex: strPtr(productNameRegex)},
				{Key: "skuName", Value: strPtr(skuName)},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	}
}

func (r *MSSQLDatabase) serverlessComputeHoursCostComponent() *schema.CostComponent {
	productNameRegex := fmt.Sprintf("/%s - %s/", r.Tier, r.Family)

	var vCoreHours *decimal.Decimal
	if r.MonthlyVCoreHours != nil {
		vCoreHours = decimalPtr(decimal.NewFromInt(*r.MonthlyVCoreHours))
	}

	serverlessSkuName := r.mssqlSkuName(1)
	return &schema.CostComponent{
		Name:            fmt.Sprintf("Compute (serverless, %s)", r.SKU),
		Unit:            "vCore-hours",
		UnitMultiplier:  decimal.NewFromInt(1),
		MonthlyQuantity: vCoreHours,
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(r.Region),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", ValueRegex: strPtr(productNameRegex)},
				{Key: "skuName", Value: strPtr(serverlessSkuName)},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	}
}

func (r *MSSQLDatabase) dtuPurchaseCostComponents() []*schema.CostComponent {
	var costComponents []*schema.CostComponent

	skuName := strings.ToLower(r.SKU)
	if skuName == "basic" {
		skuName = "b"
	}

	costComponents = append(costComponents, &schema.CostComponent{
		Name:           fmt.Sprintf("Compute (%s)", strings.ToTitle(r.SKU)),
		Unit:           "days",
		UnitMultiplier: decimal.NewFromInt(1),
		// This is not the same as the 730h/month value we use elsewhere but it looks more understandable than seeing `30.4166` in the output
		MonthlyQuantity: decimalPtr(decimal.NewFromInt(30)),
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(r.Region),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", ValueRegex: strPtr("/^SQL Database Single/i")},
				{Key: "skuName", ValueRegex: strPtr(fmt.Sprintf("/^%s$/i", skuName))},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	})

	if skuName != "b" {
		sn := map[string]string{
			"p": "Premium",
			"s": "Standard",
		}[skuName[:1]]

		var storageGB *decimal.Decimal
		if r.MaxSizeGB != nil {
			storageGB = decimalPtr(decimal.NewFromInt(*r.MaxSizeGB))

			if strings.ToLower(sn) == "premium" {
				storageGB = decimalPtr(storageGB.Sub(decimal.NewFromInt(500)))
			} else {
				storageGB = decimalPtr(storageGB.Sub(decimal.NewFromInt(250)))
			}

			if storageGB.IsNegative() {
				storageGB = nil
			}
		}

		if r.ExtraDataStorageGB != nil {
			storageGB = decimalPtr(decimal.NewFromInt(*r.ExtraDataStorageGB))
		}

		costComponents = append(costComponents, &schema.CostComponent{
			Name:            "Extra data storage",
			Unit:            "GB",
			UnitMultiplier:  decimal.NewFromInt(1),
			MonthlyQuantity: storageGB,
			ProductFilter: &schema.ProductFilter{
				VendorName:    strPtr(vendorName),
				Region:        strPtr(r.Region),
				Service:       strPtr(sqlServiceName),
				ProductFamily: strPtr(sqlProductFamily),
				AttributeFilters: []*schema.AttributeFilter{
					{Key: "productName", ValueRegex: strPtr(fmt.Sprintf("/SQL Database %s - Storage/i", sn))},
					{Key: "skuName", ValueRegex: strPtr(fmt.Sprintf("/^%s$/i", sn))},
					{Key: "meterName", Value: strPtr("Data Stored")},
				},
			},
			PriceFilter: &schema.PriceFilter{
				PurchaseOption: strPtr("Consumption"),
			},
		})
	}

	costComponents = append(costComponents, r.longTermRetentionMSSQLCostComponent())

	return costComponents
}

func (r *MSSQLDatabase) mssqlSkuName(cores int64) string {
	sku := fmt.Sprintf("%d vCore", cores)

	if r.ZoneRedundant {
		sku += " Zone Redundancy"
	}
	return sku
}

func (r *MSSQLDatabase) sqlLicenseCostComponent() *schema.CostComponent {
	licenseRegion := "Global"
	if strings.Contains(r.Region, "usgov") {
		licenseRegion = "US Gov"
	}

	if strings.Contains(r.Region, "china") {
		licenseRegion = "China"
	}

	if strings.Contains(r.Region, "germany") {
		licenseRegion = "Germany"
	}

	return &schema.CostComponent{
		Name:           "SQL license",
		Unit:           "vCore-hours",
		UnitMultiplier: decimal.NewFromInt(1),
		HourlyQuantity: decimalPtr(decimal.NewFromInt(r.Cores)),
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(licenseRegion),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", ValueRegex: strPtr(fmt.Sprintf("/%s - %s/", r.Tier, "SQL License"))},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	}
}

func (r *MSSQLDatabase) mssqlStorageComponent() *schema.CostComponent {
	storageGB := decimalPtr(decimal.NewFromInt(5))
	if r.MaxSizeGB != nil {
		storageGB = decimalPtr(decimal.NewFromInt(*r.MaxSizeGB))
	}

	storageTier := r.Tier
	if strings.ToLower(storageTier) == "general purpose - serverless" {
		storageTier = "General Purpose"
	}

	skuName := storageTier
	if r.ZoneRedundant {
		skuName += " Zone Redundancy"
	}

	productNameRegex := fmt.Sprintf("/%s - Storage/", storageTier)

	return &schema.CostComponent{
		Name:            "Storage",
		Unit:            "GB",
		UnitMultiplier:  decimal.NewFromInt(1),
		MonthlyQuantity: storageGB,
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(r.Region),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", ValueRegex: strPtr(productNameRegex)},
				{Key: "skuName", Value: strPtr(skuName)},
				{Key: "meterName", ValueRegex: strPtr("/^Data Stored/")},
			},
		},
	}
}

func (r *MSSQLDatabase) longTermRetentionMSSQLCostComponent() *schema.CostComponent {
	var retention *decimal.Decimal
	if r.LongTermRetentionStorageGB != nil {
		retention = decimalPtr(decimal.NewFromInt(*r.LongTermRetentionStorageGB))
	}

	return &schema.CostComponent{
		Name:            "Long-term retention",
		Unit:            "GB",
		UnitMultiplier:  decimal.NewFromInt(1),
		MonthlyQuantity: retention,
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr(vendorName),
			Region:        strPtr(r.Region),
			Service:       strPtr(sqlServiceName),
			ProductFamily: strPtr(sqlProductFamily),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", Value: strPtr("SQL Database - LTR Backup Storage")},
				{Key: "skuName", Value: strPtr("Backup RA-GRS")},
				{Key: "meterName", Value: strPtr("RA-GRS Data Stored")},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	}
}
