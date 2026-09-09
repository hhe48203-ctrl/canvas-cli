package cmd

import "github.com/spf13/cobra"

func newPagesCommand() *cobra.Command {
	pages := &cobra.Command{
		Use: "pages", Short: "Read course pages",
		Example: `  canvas pages show 123 course-overview --json`,
	}
	show := getResourceCommand("show COURSE_ID PAGE_URL", "Show a Canvas page", "/api/v1/courses/%s/pages/%s")
	show.Long = "Show a Canvas page. PAGE_URL is the Canvas page identifier or slug, not an external URL."
	show.Example = `  canvas pages show 123 course-overview --json`
	pages.AddCommand(show)
	return pages
}
