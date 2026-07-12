package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

func newCoursesCommand() *cobra.Command {
	courses := &cobra.Command{
		Use: "courses", Short: "List and inspect Canvas courses",
		Example: `  canvas courses list --query enrollment_type=student
  canvas courses list --all-pages --json
  canvas courses show 12345 --query include[]=term`,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "List courses for the current user",
		Long:  "List courses visible to the authenticated user. Canvas paginates lists; use --all-pages to follow every opaque Link header.",
		Example: `  canvas courses list
  canvas courses list --query enrollment_type=student --query include[]=term
  canvas courses list --all-pages --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getAndEmit("/api/v1/courses")
		},
	}
	addReadFlags(list, true)
	show := &cobra.Command{
		Use:   "show COURSE_ID",
		Short: "Show a course",
		Args:  cobra.ExactArgs(1),
		Example: `  canvas courses show 12345
  canvas courses show 12345 --query include[]=term --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getAndEmit("/api/v1/courses/" + url.PathEscape(args[0]))
		},
	}
	addReadFlags(show, false)
	courses.AddCommand(
		list,
		show,
	)
	return courses
}

func getAndEmit(path string) error {
	ctx, c, err := contextWithClient()
	if err != nil {
		return err
	}
	resp, err := c.Request(ctx, http.MethodGet, path, parsePairs(queryArgs), nil, "")
	if err != nil {
		return err
	}
	return emitHTTPResponse(ctx, c, resp)
}

func getResourceCommand(use, short, path string) *cobra.Command {
	command := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  short + ". Use --query for Canvas query parameters and --all-pages on list commands to retrieve every page.",
		Args:  cobra.ExactArgs(strings.Count(path, "%s")),
		RunE: func(cmd *cobra.Command, args []string) error {
			values := make([]any, len(args))
			for i, value := range args {
				values[i] = url.PathEscape(value)
			}
			return getAndEmit(fmt.Sprintf(path, values...))
		},
	}
	addReadFlags(command, strings.HasPrefix(use, "list ") || use == "list")
	return command
}

func addReadFlags(cmd *cobra.Command, paginated bool) {
	cmd.Flags().StringArrayVar(&queryArgs, "query", nil, "Canvas query parameter key=value; repeat for arrays such as include[]=term")
	cmd.Flags().BoolVar(&includeHeaders, "include-headers", false, "Include HTTP status and response headers in the output envelope")
	if paginated {
		cmd.Flags().BoolVar(&allPages, "all-pages", false, "Follow Canvas Link headers and combine every result page")
	}
}
