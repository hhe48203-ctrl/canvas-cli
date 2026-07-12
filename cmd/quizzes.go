package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

var (
	accessCode  string
	answersFile string
	attempt     int
	validation  string
)

func newQuizzesCommand() *cobra.Command {
	quizzes := &cobra.Command{
		Use: "quizzes", Short: "Inspect and take classic Canvas quizzes",
		Long: "Work with Classic Quizzes. New Quizzes uses separate services and can be reached through 'canvas api invoke'. Starting, answering, and completing a quiz are writes and require --confirm.",
		Example: `  canvas quizzes list 123 --all-pages
  canvas quizzes start 123 456 --confirm
  canvas quizzes questions 789
  canvas quizzes answer 789 --answers-file answers.json --confirm
  canvas quizzes complete 123 456 789 --attempt 1 --validation-token TOKEN --confirm`,
	}
	list := getResourceCommand("list COURSE_ID", "List Classic Quizzes", "/api/v1/courses/%s/quizzes")
	list.Example = `  canvas quizzes list 123 --all-pages
  canvas quizzes list 123 --query search_term=midterm --json`
	show := getResourceCommand("show COURSE_ID QUIZ_ID", "Show a Classic Quiz", "/api/v1/courses/%s/quizzes/%s")
	show.Example = `  canvas quizzes show 123 456 --json`
	quizzes.AddCommand(
		list,
		show,
		newStartQuizCommand(),
		newQuizQuestionsCommand(),
		newAnswerQuizCommand(),
		newCompleteQuizCommand(),
	)
	return quizzes
}

func newStartQuizCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "start COURSE_ID QUIZ_ID", Short: "Start a quiz submission", Args: cobra.ExactArgs(2),
		Long: "Create a Classic Quiz submission. This may consume an allowed attempt and therefore requires --confirm.",
		Example: `  canvas quizzes start 123 456 --confirm
  canvas quizzes start 123 456 --access-code CODE --confirm --json`,
		PreRunE: func(cmd *cobra.Command, args []string) error { return requireConfirm() },
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			values := url.Values{}
			if accessCode != "" {
				values.Set("access_code", accessCode)
			}
			resp, err := c.Form(ctx, http.MethodPost, fmt.Sprintf("/api/v1/courses/%s/quizzes/%s/submissions", url.PathEscape(args[0]), url.PathEscape(args[1])), values)
			if err != nil {
				return err
			}
			return emit(decodeJSON(resp.Body))
		},
	}
	cmd.Flags().StringVar(&accessCode, "access-code", "", "Quiz access code")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm creation of a quiz attempt")
	return cmd
}

func newQuizQuestionsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "questions SUBMISSION_ID", Short: "List questions for a quiz submission", Args: cobra.ExactArgs(1),
		Long: "List questions for an active Classic Quiz submission. Use --all-pages when the quiz contains more than one result page.",
		Example: `  canvas quizzes questions 789 --all-pages
  canvas quizzes questions 789 --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return getAndEmit("/api/v1/quiz_submissions/" + url.PathEscape(args[0]) + "/questions")
		},
	}
	addReadFlags(cmd, true)
	return cmd
}

func newAnswerQuizCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "answer SUBMISSION_ID", Short: "Submit answers from a JSON file", Args: cobra.ExactArgs(1),
		Long: `Submit one or more Classic Quiz answers from JSON. Scalar, array, and
nested answer values are encoded using Canvas' bracketed form convention. This
updates the active attempt and requires --confirm.`,
		Example: `  canvas quizzes answer 789 --answers-file answers.json --confirm

  # answers.json
  # {"attempt":1,"validation_token":"TOKEN","quiz_questions":[
  #   {"id":101,"answer":"A"},
  #   {"id":102,"answer":["choice-1","choice-3"]}
  # ]}`,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if answersFile == "" {
				return fmt.Errorf("--answers-file is required")
			}
			return requireConfirm()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(answersFile)
			if err != nil {
				return err
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				return err
			}
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			values := url.Values{}
			for _, key := range []string{"attempt", "validation_token", "access_code"} {
				if value, ok := payload[key]; ok {
					values.Set(key, fmt.Sprint(value))
				}
			}
			questions, ok := payload["quiz_questions"].([]any)
			if !ok {
				return fmt.Errorf("answers file must contain quiz_questions array")
			}
			for i, question := range questions {
				q, ok := question.(map[string]any)
				if !ok {
					return fmt.Errorf("quiz_questions[%d] must be an object", i)
				}
				id, ok := q["id"]
				if !ok {
					return fmt.Errorf("quiz_questions[%d].id is required", i)
				}
				answer, ok := q["answer"]
				if !ok {
					return fmt.Errorf("quiz_questions[%d].answer is required", i)
				}
				values.Set(fmt.Sprintf("quiz_questions[%d][id]", i), fmt.Sprint(id))
				if err := addFormValue(values, fmt.Sprintf("quiz_questions[%d][answer]", i), answer); err != nil {
					return fmt.Errorf("quiz_questions[%d].answer: %w", i, err)
				}
			}
			resp, err := c.Form(ctx, http.MethodPost, "/api/v1/quiz_submissions/"+url.PathEscape(args[0])+"/questions", values)
			if err != nil {
				return err
			}
			return emit(decodeJSON(resp.Body))
		},
	}
	cmd.Flags().StringVar(&answersFile, "answers-file", "", "JSON file with attempt, validation_token, and quiz_questions")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm updating answers in the active quiz attempt")
	return cmd
}

func newCompleteQuizCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "complete COURSE_ID QUIZ_ID SUBMISSION_ID", Short: "Complete and submit a quiz", Args: cobra.ExactArgs(3),
		Long: "Complete an active Classic Quiz submission. This is irreversible and requires --confirm.",
		Example: `  canvas quizzes complete 123 456 789 \
    --attempt 1 --validation-token TOKEN --confirm`,
		PreRunE: func(cmd *cobra.Command, args []string) error { return requireConfirm() },
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			values := url.Values{}
			if attempt > 0 {
				values.Set("attempt", fmt.Sprint(attempt))
			}
			if validation != "" {
				values.Set("validation_token", validation)
			}
			resp, err := c.Form(ctx, http.MethodPost, fmt.Sprintf("/api/v1/courses/%s/quizzes/%s/submissions/%s/complete", url.PathEscape(args[0]), url.PathEscape(args[1]), url.PathEscape(args[2])), values)
			if err != nil {
				return err
			}
			return emit(decodeJSON(resp.Body))
		},
	}
	cmd.Flags().IntVar(&attempt, "attempt", 0, "Latest quiz attempt number")
	cmd.Flags().StringVar(&validation, "validation-token", "", "Quiz submission validation token")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm this irreversible write operation")
	return cmd
}

func addFormValue(values url.Values, key string, value any) error {
	switch typed := value.(type) {
	case nil:
		values.Set(key, "")
	case []any:
		for _, item := range typed {
			if _, nested := item.([]any); nested {
				return fmt.Errorf("nested arrays are not supported")
			}
			if _, nested := item.(map[string]any); nested {
				return fmt.Errorf("objects inside arrays are not supported")
			}
			values.Add(key+"[]", fmt.Sprint(item))
		}
	case map[string]any:
		for nestedKey, nestedValue := range typed {
			if err := addFormValue(values, key+"["+nestedKey+"]", nestedValue); err != nil {
				return err
			}
		}
	default:
		values.Set(key, fmt.Sprint(typed))
	}
	return nil
}
