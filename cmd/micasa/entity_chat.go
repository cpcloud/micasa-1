// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"strconv"

	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
)

func chatInputCols() []showCol[data.ChatInput] {
	return []showCol[data.ChatInput]{
		{
			header: "ID",
			value:  func(c data.ChatInput) string { return strconv.FormatUint(uint64(c.ID), 10) },
		},
		{header: "INPUT", value: func(c data.ChatInput) string { return c.Input }},
		{
			header: "CREATED AT",
			value:  func(c data.ChatInput) string { return c.CreatedAt.Format(data.DateLayout) },
		},
	}
}

func chatInputToMap(c data.ChatInput) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"input":      c.Input,
		"created_at": c.CreatedAt,
	}
}

func newChatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "View and manage chat history",
	}
	cmd.AddCommand(buildChatListCmd(), buildChatDeleteCmd())
	return cmd
}

func buildChatListCmd() *cobra.Command {
	var showTable bool
	cmd := &cobra.Command{
		Use:           "list [database-path]",
		Short:         "List chat history",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 0 {
				dbPath = args[0]
			}
			store, err := openExisting(dbPathFromEnvOrArgStr(dbPath))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			items, err := store.ListChatInputs()
			if err != nil {
				return err
			}

			if showTable {
				return writeTable(cmd.OutOrStdout(), "CHAT HISTORY", items, chatInputCols())
			}
			return writeJSON(cmd.OutOrStdout(), items, chatInputToMap)
		},
	}
	cmd.Flags().BoolVar(&showTable, "table", false, "Output as table")
	return cmd
}

func buildChatDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "delete <id> [database-path]",
		Short:         "Delete a chat history entry",
		Args:          cobra.RangeArgs(1, 2),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(_ *cobra.Command, args []string) error {
			var dbPath string
			if len(args) > 1 {
				dbPath = args[1]
			}
			store, err := openExisting(dbPathFromEnvOrArgStr(dbPath))
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			return store.DeleteChatInput(args[0])
		},
	}
}
