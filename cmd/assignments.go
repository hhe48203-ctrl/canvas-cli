package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/hhe48203-ctrl/canvas-cli/internal/canvas"
	"github.com/spf13/cobra"
)

var (
	assignmentFiles []string
	assignmentText  string
	assignmentURL   string
	comment         string
)

type assignmentSubmissionPreview struct {
	DryRun         bool                    `json:"dry_run" yaml:"dry_run"`
	CourseID       string                  `json:"course_id" yaml:"course_id"`
	AssignmentID   string                  `json:"assignment_id" yaml:"assignment_id"`
	Target         string                  `json:"target" yaml:"target"`
	SubmissionType string                  `json:"submission_type" yaml:"submission_type"`
	Files          []assignmentPreviewFile `json:"files,omitempty" yaml:"files,omitempty"`
	Text           string                  `json:"text,omitempty" yaml:"text,omitempty"`
	URL            string                  `json:"url,omitempty" yaml:"url,omitempty"`
	Comment        string                  `json:"comment,omitempty" yaml:"comment,omitempty"`
}

type assignmentPreviewFile struct {
	Name              string `json:"name" yaml:"name"`
	Size              int64  `json:"size" yaml:"size"`
	UploadRequired    bool   `json:"upload_required" yaml:"upload_required"`
	RemoteIDAvailable bool   `json:"remote_id_available" yaml:"remote_id_available"`
}

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
		Short: "Submit text, a URL, or files to an assignment",
		Long:  "Submit exactly one mode: one or more --file flags, --text, or --url. File submissions first complete Canvas' multi-stage upload workflow. This write requires --confirm; use --dry-run to preview without uploading.",
		Example: `  canvas assignments submit 123 456 --file ./answer.pdf --file ./appendix.pdf --confirm
  canvas assignments submit 123 456 --text '<p>My answer</p>' --confirm
  canvas assignments submit 123 456 --url https://example.com/report --comment 'Final version' --confirm`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			count := 0
			if len(assignmentFiles) > 0 {
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
			if !dryRun {
				if err := requireConfirm(); err != nil {
					return err
				}
			}
			_, err := preflightAssignmentFiles(assignmentFiles)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun {
				preview, err := previewAssignmentSubmission(args)
				if err != nil {
					return err
				}
				return emit(preview)
			}
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			submissionPath := assignmentSubmissionPath(args[0], args[1])
			values := url.Values{}
			if comment != "" {
				values.Set("comment[text_comment]", comment)
			}
			if len(assignmentFiles) > 0 {
				endpoint := submissionPath + "/files"
				fileIDs, uploadErr := collectUploadedFileIDs(assignmentFiles, func(filePath string) (map[string]any, error) {
					return c.Upload(ctx, endpoint, filePath)
				})
				if uploadErr != nil {
					return uploadErr
				}
				values.Set("submission[submission_type]", "online_upload")
				for _, fileID := range fileIDs {
					values.Add("submission[file_ids][]", fileID)
				}
			} else if assignmentText != "" {
				values.Set("submission[submission_type]", "online_text_entry")
				values.Set("submission[body]", assignmentText)
			} else {
				values.Set("submission[submission_type]", "online_url")
				values.Set("submission[url]", assignmentURL)
			}
			resp, err := c.Form(ctx, http.MethodPost, submissionPath, values)
			if err != nil {
				return err
			}
			return emit(decodeJSON(resp.Body))
		},
	}
	cmd.Flags().StringArrayVar(&assignmentFiles, "file", nil, "File to upload; repeat to submit multiple files")
	cmd.Flags().StringVar(&assignmentText, "text", "", "HTML/text entry")
	cmd.Flags().StringVar(&assignmentURL, "url", "", "Submission URL")
	cmd.Flags().StringVar(&comment, "comment", "", "Optional submission comment")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm this write operation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview this submission without confirming, uploading, or sending it")
	return cmd
}

func preflightAssignmentFiles(filePaths []string) ([]os.FileInfo, error) {
	files := make([]os.FileInfo, 0, len(filePaths))
	for _, filePath := range filePaths {
		info, err := canvas.ValidateUploadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", filePath, err)
		}
		files = append(files, info)
	}
	return files, nil
}

func previewAssignmentSubmission(args []string) (assignmentSubmissionPreview, error) {
	preview := assignmentSubmissionPreview{DryRun: true, CourseID: args[0], AssignmentID: args[1], Target: assignmentSubmissionPath(args[0], args[1]), Comment: comment}
	if len(assignmentFiles) > 0 {
		infos, err := preflightAssignmentFiles(assignmentFiles)
		if err != nil {
			return assignmentSubmissionPreview{}, err
		}
		preview.SubmissionType = "online_upload"
		preview.Files = make([]assignmentPreviewFile, len(infos))
		for i, info := range infos {
			preview.Files[i] = assignmentPreviewFile{Name: filepath.Base(assignmentFiles[i]), Size: info.Size(), UploadRequired: true}
		}
	} else if assignmentText != "" {
		preview.SubmissionType, preview.Text = "online_text_entry", assignmentText
	} else {
		preview.SubmissionType, preview.URL = "online_url", assignmentURL
	}
	return preview, nil
}

func assignmentSubmissionPath(courseID, assignmentID string) string {
	return fmt.Sprintf("/api/v1/courses/%s/assignments/%s/submissions/self", url.PathEscape(courseID), url.PathEscape(assignmentID))
}

func collectUploadedFileIDs(filePaths []string, upload func(string) (map[string]any, error)) ([]string, error) {
	fileIDs := make([]string, 0, len(filePaths))
	for _, filePath := range filePaths {
		fileData, err := upload(filePath)
		if err != nil {
			return nil, fmt.Errorf("upload %q: %w", filePath, err)
		}
		fileID := extractID(fileData)
		if fileID == "" {
			return nil, fmt.Errorf("upload %q: Canvas did not return a file id", filePath)
		}
		fileIDs = append(fileIDs, fileID)
	}
	return fileIDs, nil
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
