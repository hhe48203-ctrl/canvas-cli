package cmd

import (
	"fmt"

	"github.com/hhe48203-ctrl/canvas-cli/internal/config"
	"github.com/spf13/cobra"
)

func newAuthCommand() *cobra.Command {
	auth := &cobra.Command{
		Use: "auth", Short: "Configure and verify Canvas authentication",
		Long: "Configure the Canvas instance URL and verify CANVAS_API_TOKEN. Tokens are read only from the environment and are never written to disk.",
		Example: `  canvas auth set-url https://school.instructure.com
  export CANVAS_API_TOKEN=token
  canvas auth status --json`,
	}
	auth.AddCommand(
		&cobra.Command{
			Use:   "status",
			Short: "Check whether the configured token works",
			Long:  "Call /api/v1/users/self to verify the configured base URL and CANVAS_API_TOKEN.",
			Example: `  canvas auth status
  CANVAS_BASE_URL=https://school.instructure.com CANVAS_API_TOKEN=token canvas auth status --json`,
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx, c, err := contextWithClient()
				if err != nil {
					return err
				}
				resp, err := c.Request(ctx, "GET", "/api/v1/users/self", nil, nil, "")
				if err != nil {
					return err
				}
				return emit(map[string]any{"authenticated": true, "user": decodeJSON(resp.Body)})
			},
		},
		&cobra.Command{
			Use:     "set-url URL",
			Short:   "Save a Canvas instance URL",
			Long:    "Save the Canvas instance base URL in the user configuration file. The access token is not stored.",
			Example: `  canvas auth set-url https://school.instructure.com`,
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := config.SaveBaseURL(args[0]); err != nil {
					return err
				}
				fmt.Println("Canvas URL saved")
				return nil
			},
		},
	)
	return auth
}

func newMeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the current Canvas user",
		Long:  "Return the Canvas user associated with CANVAS_API_TOKEN.",
		Example: `  canvas me
  canvas me --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, c, err := contextWithClient()
			if err != nil {
				return err
			}
			resp, err := c.Request(ctx, "GET", "/api/v1/users/self", nil, nil, "")
			if err != nil {
				return err
			}
			return emit(decodeJSON(resp.Body))
		},
	}
}
