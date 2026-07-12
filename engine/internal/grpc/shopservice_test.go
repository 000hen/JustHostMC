package grpcsvc

import (
	"testing"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/scripting"
)

func TestProjectToProtoMapsDistribution(t *testing.T) {
	tests := []struct {
		name string
		in   scripting.ShopDistribution
		want mcmanagerv1.ShopDistribution
	}{
		{
			name: "unknown",
			in:   scripting.ShopDistributionUnknown,
			want: mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_UNKNOWN,
		},
		{
			name: "direct",
			in:   scripting.ShopDistributionDirect,
			want: mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_DIRECT,
		},
		{
			name: "website",
			in:   scripting.ShopDistributionWebsiteOnly,
			want: mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_WEBSITE_ONLY,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectToProto("shop", scripting.ShopProject{Distribution: tt.in})
			if got.Distribution != tt.want {
				t.Fatalf("distribution = %v, want %v", got.Distribution, tt.want)
			}
		})
	}
}

func TestCategoriesToProto(t *testing.T) {
	in := []scripting.ShopCategory{{
		ID:              "409",
		Name:            "Technology",
		Slug:            "technology",
		LocalizationKey: "shop.category.curseforge.technology",
	}}
	got := categoriesToProto(in)
	if len(got) != 1 || got[0].Id != "409" || got[0].Name != "Technology" ||
		got[0].Slug != "technology" ||
		got[0].LocalizationKey != "shop.category.curseforge.technology" {
		t.Fatalf("categories = %#v", got)
	}
}
