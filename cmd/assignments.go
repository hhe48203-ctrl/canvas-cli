package cmd

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

var (
	assignmentFile string
	assignmentText string
	assignmentURL  string
	comment        string
)

func newAssignmentsCommand() *cobra.Command {
	assignments := &cobra.Command{
		Use: "assignments", Short: "List, inspect, and submit assignments",
		Example: `  canvas assignments list 123 --all-pages
  canvas assignments show 123 456
  canvas assignments submit 123 456 --file ./homework.pdf --confirm`,
	}
	list := getResourceCommand("list COURSE_ID", "List assignments in a course", "/api/v1/courses/%s/assignments")
	list.Example = `  canvas assignments list 123 --query order_by=due_at
  canvas assignments list 123 --query 'include[]=submission' --all-pages --json`
	show := getResourceCommand("show COURSE_ID ASSIGNMENT_ID", "Show an assignment", "/api/v1/courses/%s/assignments/%s")
	show.Example = `  canvas assignments show 123 456
  canvas assignments show 123 456 --query 'include[]=submission' --json`
	assignments.AddCommand(list, show, newSubmitAssignmentCommand())
	return assignments
}

func newSubmitAssignmentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit COURSE_ID ASSIGNMENT_ID",
		Short: "Submit text, a URL, or a file to an assignment",
		Long:  "Submit exactly one of --file, --text, or --url. File submissions first complete Canvas' multi-stage upload workflow. This write requires --confirm.",
		Example: `  canvas assignments submit 123 456 --file ./homework.pdf --confirm
  canvas assignments submit 123 456 --text '<p>My answer</p>' --confirm
  canvas assignments submit 123 456 --url https://example.com/report --comment 'Final version' --confirm`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			count := 0
			if assignmentFile != "" {
				count++
			}
			if assignmentText != "" {
				count++
			}
			if assignmentURL != "" {
				count++
			}
			if count != 1 {
				return fmt.Errorf("provide exactly one of --file, --text, or --url")
			}
			return requireConfirm()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			courseID, assignmentID := url.PathEscape(args[0]), url.PathEscape(args[1])
			values := url.Values{}
			if comment != "" {
				values.Set("comment[text_comment]", comment)
			}
			if assignmentFile != "" {
				endpoint := fmt.Sprintf("/api/v1/courses/%s/assignments/%s/submissions/self/files", courseID, assignmentID)
				fileData, uploadErr := c.Upload(ctx, endpoint, assignmentFile)
				if uploadErr != nil {
					return uploadErr
				}
				fileID := extractID(fileData)
				if fileID == "" {
					return fmt.Errorf("Canvas upload did not return a file id")
				}
				values.Set("submission[submission_type]", "online_upload")
				values.Set("submission[file_ids][]", fileID)
			} else if assignmentText != "" {
				values.Set("submission[submission_type]", "online_text_entry")
				values.Set("submission[body]", assignmentText)
			} else {
				values.Set("submission[submission_type]", "online_url")
				values.Set("submission[url]", assignmentURL)
			}
			resp, err := c.Form(ctx, http.MethodPost, fmt.Sprintf("/api/v1/courses/%s/assignments/%s/submissions/self", courseID, assignmentID), values)
			if err != nil {
				return err
			}
			return emit(decodeJSON(resp.Body))
		},
	}
	cmd.Flags().StringVar(&assignmentFile, "file", "", "File to upload")
	cmd.Flags().StringVar(&assignmentText, "text", "", "HTML/text entry")
	cmd.Flags().StringVar(&assignmentURL, "url", "", "Submission URL")
	cmd.Flags().StringVar(&comment, "comment", "", "Optional submission comment")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm this write operation")
	return cmd
}

func extractID(data map[string]any) string {
	for _, key := range []string{"id", "file_id"} {
		if value, ok := data[key]; ok {
			return fmt.Sprint(value)
		}
	}
	if nested, ok := data["file"].(map[string]any); ok {
		if value, ok := nested["id"]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}
