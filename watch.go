// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 Hajime Hoshi

package ssg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fswatcher/fswatcher"
)

func watch(ctx context.Context, options *GenerateOptions, state *serveState) error {
	if options == nil || options.SiteName == "" {
		return fmt.Errorf("ssg: SiteName must not be empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	w, err := fswatcher.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := w.AddRecursive(inDir, fswatcher.All); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-w.Events:
			if !ok {
				return nil
			}
			slog.Info("Regenerating site")
			if err := state.regenerate(options); err != nil {
				return err
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			return err
		}
	}
}
