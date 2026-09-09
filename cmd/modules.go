package cmd

import "github.com/spf13/cobra"

func newModulesCommand() *cobra.Command {
	modules := &cobra.Command{
		Use: "modules", Short: "List course modules",
		Example: `  canvas modules list 123 --all-pages
  canvas modules list 123 --query 'include[]=items' --json`,
	}
	list := getResourceCommand("list COURSE_ID", "List modules in a course", "/api/v1/courses/%s/modules")
	list.Example = `  canvas modules list 123 --all-pages
  canvas modules list 123 --query 'include[]=items' --json`
	modules.AddCommand(list)
	return modules
}
