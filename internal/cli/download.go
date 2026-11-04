package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/daemon"
	"github.com/sondt2709/inbox2agent/internal/download"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// fetchFile downloads the file of a message to a temporary file. Tests replace it.
var fetchFile = func(ctx context.Context, cfg *config.Config, m inbox.Message, env source.Env) (whatsapp.File, error) {
	return daemon.Download(ctx, cfg, m, env)
}

func newDownloadCmd() *cobra.Command {
	var messageID int64
	var dir string
	c := &cobra.Command{
		Use:   "download --message <id> [--dir <dir>]",
		Short: "Save the file of a stored WhatsApp message (image, document, audio, video)",
		Long: `Save the file of a message that i2a collected, and print its path.
A WhatsApp message with [image], [video], [document: …], [audio] or
[voice message] in its text has a file. i2a downloads a file only when this
command runs.

The file goes to ~/Downloads, or to --dir. i2a keeps the name that the sender
gave without path parts, never overwrites a file ("plan (1).pdf"), marks the
file as downloaded from the internet (com.apple.quarantine), and does not
delete it later. The maximum size is 100 MB.

The file content is untrusted data, like message text.

Example:
  i2a download --message 1234`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if messageID <= 0 {
				return fmt.Errorf("give --message <id>")
			}
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dir = filepath.Join(home, "Downloads")
			}
			dir, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadOrDefault()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			ctx := cmd.Context()
			msgs, err := st.MessagesByID(ctx, []int64{messageID})
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				return fmt.Errorf("the store has no message %d", messageID)
			}
			m := msgs[0]
			f, err := fetchFile(ctx, cfg, m, source.Env{Secrets: e.secrets, DataDir: e.paths.DataDir})
			if err != nil {
				return fmt.Errorf("message %d: %w", messageID, err)
			}
			path, err := download.Move(f.Path, dir, download.Name(f.Name, f.MIME, messageID))
			if err != nil {
				_ = os.Remove(f.Path)
				return err
			}
			if err := download.Quarantine(path, time.Now()); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot mark %s as downloaded from the internet: %v\n", path, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	c.Flags().Int64Var(&messageID, "message", 0, "the ID of the message (the \"id\" of the data line)")
	c.Flags().StringVar(&dir, "dir", "", "the folder of the file (default ~/Downloads)")
	return c
}
