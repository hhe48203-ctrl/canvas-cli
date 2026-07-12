package cmd

import (
	"net/url"
	"path/filepath"

	"github.com/spf13/cobra"
)

var downloadPath string

func newFilesCommand() *cobra.Command {
	files := &cobra.Command{
		Use: "files", Short: "List, download, and upload Canvas files",
		Long: "Work with course files. List results are paginated; uploads use Canvas' authenticated three-step file workflow.",
		Example: `  canvas files list 123 --all-pages
  canvas files download 456 --destination ./lecture.pdf
  canvas files upload 123 ./notes.pdf --confirm`,
	}
	list := getResourceCommand("list COURSE_ID", "List files in a course", "/api/v1/courses/%s/files")
	list.Example = `  canvas files list 123 --query search_term=lecture
  canvas files list 123 --query 'include[]=usage_rights' --all-pages --json`
	download := &cobra.Command{
		Use:   "download FILE_ID",
		Short: "Download a file by ID",
		Long:  "Download a Canvas file to --destination. If omitted, the file is saved as canvas-file-FILE_ID in the current directory.",
		Args:  cobra.ExactArgs(1),
		Example: `  canvas files download 456
  canvas files download 456 --destination ./lecture.pdf --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			path := downloadPath
			if path == "" {
				path = "canvas-file-" + args[0]
			}
			bytes, err := c.Download(ctx, "/api/v1/files/"+url.PathEscape(args[0])+"/download", path)
			if err != nil {
				return err
			}
			return emit(map[string]any{"path": filepath.Clean(path), "bytes": bytes})
		},
	}
	download.Flags().StringVarP(&downloadPath, "destination", "o", "", "Local destination path (default canvas-file-FILE_ID)")
	files.AddCommand(
		list,
		newUploadFileCommand(),
		download,
	)
	return files
}

func newUploadFileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "upload COURSE_ID FILE", Short: "Upload a file to a course", Args: cobra.ExactArgs(2),
		Long: "Upload a local file with Canvas' initialize, multipart transfer, and authenticated confirmation workflow.",
		Example: `  canvas files upload 123 ./notes.pdf --confirm
  canvas files upload 123 ./data.csv --confirm --json`,
		PreRunE: func(cmd *cobra.Command, args []string) error { return requireConfirm() },
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			data, err := c.Upload(ctx, "/api/v1/courses/"+url.PathEscape(args[0])+"/files", args[1])
			if err != nil {
				return err
			}
			return emit(data)
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm this write operation")
	return cmd
}
