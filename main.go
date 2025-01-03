package main

import (
	"github.com/AlehYarmalovich/terraform-provider-cachefly/cachefly"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: cachefly.Provider,
	})
}
