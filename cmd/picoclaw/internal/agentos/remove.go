package agentos

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newRemoveCommand cria o comando para remover um sistema
func newRemoveCommand() *cobra.Command {
	var (
		dataDir   string
		system    string
		force     bool
		keepData  bool
	)

	cmd := &cobra.Command{
		Use:   "remove [system]",
		Short: "Remove um sistema do AgentOS",
		Long: `Remove um sistema registrado do AgentOS.

Por padrão, este comando remove:
- O registro do sistema no agentos_registry.json
- O banco de dados do sistema
- Os arquivos de configuração

Use --keep-data para preservar os dados do banco de dados.
Use --force para remover sem confirmação.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir = getDataDir(dataDir)

			// Get system name from args or flag
			if len(args) > 0 {
				system = args[0]
			}

			if system == "" {
				return fmt.Errorf("system name is required")
			}

			// Load registry
			registry, err := LoadRegistry(dataDir)
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}

			// Check if system exists
			sysInfo, err := registry.GetSystem(system)
			if err != nil {
				return fmt.Errorf("system not found: %s", system)
			}

			// Show system info
			fmt.Printf("=== System to Remove ===\n")
			fmt.Printf("Name: %s\n", sysInfo.Name)
			fmt.Printf("Database: %s\n", sysInfo.DBConnection)
			fmt.Printf("Manifest: %s\n", sysInfo.ManifestPath)
			fmt.Println()

			// Confirm removal
			if !force {
				reader := bufio.NewReader(os.Stdin)
				fmt.Printf("Are you sure you want to remove this system? [y/N]: ")
				response, _ := reader.ReadString('\n')
				response = strings.TrimSpace(strings.ToLower(response))
				if response != "y" && response != "yes" {
					fmt.Println("Removal cancelled.")
					return nil
				}
			}

			// Remove database if not keeping data
			if !keepData {
				if _, err := os.Stat(sysInfo.DBConnection); err == nil {
					fmt.Printf("Removing database: %s\n", sysInfo.DBConnection)
					if err := os.Remove(sysInfo.DBConnection); err != nil {
						fmt.Printf("Warning: failed to remove database: %v\n", err)
					}
				}

				// Also remove -shm and -wal files if they exist
				shmFile := sysInfo.DBConnection + "-shm"
				walFile := sysInfo.DBConnection + "-wal"
				os.Remove(shmFile)
				os.Remove(walFile)
			}

			// Remove system from registry
			fmt.Printf("Removing system from registry...\n")
			if err := registry.RemoveSystem(system); err != nil {
				return fmt.Errorf("failed to remove system from registry: %w", err)
			}

			// Save registry
			if err := registry.Save(dataDir); err != nil {
				return fmt.Errorf("failed to save registry: %w", err)
			}

			fmt.Printf("\nSystem '%s' removed successfully.\n", system)

			// Show remaining systems
			remaining := registry.GetSystemNames()
			if len(remaining) > 0 {
				fmt.Printf("\nRemaining systems: %s\n", strings.Join(remaining, ", "))
			} else {
				fmt.Println("\nNo systems remaining.")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&dataDir, "data-dir", "", "Data directory for AgentOS")
	cmd.Flags().StringVarP(&system, "system", "s", "", "System name to remove")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Remove without confirmation")
	cmd.Flags().BoolVar(&keepData, "keep-data", false, "Keep database files")

	return cmd
}
