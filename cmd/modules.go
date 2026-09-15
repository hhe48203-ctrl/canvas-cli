package cmd

import "github.com/spf13/cobra"

func newModulesCommand() *cobra.Command {
	modules := &cobra.Command{
		Use:   "modules",
		Short: "Read course modules and their items",
		Long: `List course modules. Canvas can omit inline items requested with include[]=items for large modules;
use 'canvas modules items list COURSE_ID MODULE_ID --all-pages' to retrieve every item.`,
		Example: `  canvas modules list 123 --all-pages
	canvas modules list 123 --query 'include[]=items' --json
  canvas modules items list 123 456 --all-pages --json`,
	}
	list := getResourceCommand("list COURSE_ID", "List modules in a course", "/api/v1/courses/%s/modules")
	list.Long = modules.Long
	list.Example = `  canvas modules list 123 --all-pages
  canvas modules list 123 --query 'include[]=items' --json`
	items := &cobra.Command{
		Use:     "items",
		Short:   "List items in a module",
		Example: "  canvas modules items list 123 456 --all-pages --json",
	}
	itemsList := getResourceCommand("list COURSE_ID MODULE_ID", "List items in a course module", "/api/v1/courses/%s/modules/%s/items")
	itemsList.Long = "List every item in a course module. Unlike include[]=items on modules list, this endpoint remains available when Canvas omits inline items."
	itemsList.Example = "  canvas modules items list 123 456 --all-pages --json"
	items.AddCommand(itemsList)
	modules.AddCommand(list, items)
	return modules
}
