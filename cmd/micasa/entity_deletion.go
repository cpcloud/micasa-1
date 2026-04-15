// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"github.com/micasa-dev/micasa/internal/data"
	"github.com/spf13/cobra"
)

func deletionRecordCols() []showCol[data.DeletionRecord] {
	return []showCol[data.DeletionRecord]{
		{header: "ID", value: func(d data.DeletionRecord) string { return d.ID }},
		{header: "ENTITY", value: func(d data.DeletionRecord) string { return d.Entity }},
		{header: "TARGET ID", value: func(d data.DeletionRecord) string { return d.TargetID }},
		{
			header: "DELETED AT",
			value:  func(d data.DeletionRecord) string { return d.DeletedAt.Format(data.DateLayout) },
		},
		{header: "RESTORED AT", value: func(d data.DeletionRecord) string {
			if d.RestoredAt != nil {
				return d.RestoredAt.Format(data.DateLayout)
			}
			return ""
		}},
	}
}

func deletionRecordToMap(d data.DeletionRecord) map[string]any {
	m := map[string]any{
		"id":         d.ID,
		"entity":     d.Entity,
		"target_id":  d.TargetID,
		"deleted_at": d.DeletedAt,
	}
	if d.RestoredAt != nil {
		m["restored_at"] = *d.RestoredAt
	} else {
		m["restored_at"] = nil
	}
	return m
}

func newDeletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deletion",
		Short: "View deletion audit records",
	}
	cmd.AddCommand(buildDeletionListCmd())
	return cmd
}

func buildDeletionListCmd() *cobra.Command {
	var showTable bool
	cmd := &cobra.Command{
		Use:           "list [database-path]",
		Short:         "List deletion records",
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

			items, err := store.ListDeletionRecords()
			if err != nil {
				return err
			}

			if showTable {
				return writeTable(
					cmd.OutOrStdout(),
					"DELETION RECORDS",
					items,
					deletionRecordCols(),
				)
			}
			return writeJSON(cmd.OutOrStdout(), items, deletionRecordToMap)
		},
	}
	cmd.Flags().BoolVar(&showTable, "table", false, "Output as table")
	return cmd
}
