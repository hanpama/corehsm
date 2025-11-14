package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/hanpama/corehsm"
)

const snapshotPath = "character_sheet.json"

// --- 1. Data (The "Model") ---
// CharacterData is the 'single source of truth' holding all character information.
type Stats struct {
	STR int `json:"str"` // Strength
	DEX int `json:"dex"` // Dexterity
	CON int `json:"con"` // Constitution
}

type CharacterData struct {
	Name      string   `json:"name"`
	Race      string   `json:"race"`
	Class     string   `json:"class"`
	Level     int      `json:"level"`
	HP        int      `json:"hp"`
	MaxHP     int      `json:"max_hp"`
	BaseStats Stats    `json:"base_stats"`
	Inventory []string `json:"inventory"`
}

// --- 2. States ---
// States manage the application's lifecycle.
var (
	RootState        = corehsm.NewState("Root", nil)
	NoSheetState     = corehsm.NewState("NoSheet", RootState)
	SheetExistsState = corehsm.NewState("SheetExists", RootState)
)

// --- 3. Command Handlers (The "Controllers") ---

// create handler works only in NoSheetState.
func create(ctx context.Context, m *corehsm.Machine[CharacterData], cmd *corehsm.Command) (corehsm.Result, error) {
	if len(cmd.Args()) != 3 {
		return corehsm.Result{}, fmt.Errorf("usage: create [name] [race] [class]")
	}
	// Create character with base stats
	m.Data = CharacterData{
		Name:      cmd.Args()[0],
		Race:      cmd.Args()[1],
		Class:     cmd.Args()[2],
		Level:     1,
		HP:        10,
		MaxHP:     10,
		BaseStats: Stats{STR: 10, DEX: 10, CON: 10},
		Inventory: []string{},
	}
	return corehsm.Result{
		Output:    fmt.Sprintf("'%s' the %s %s has been created!", m.Data.Name, m.Data.Race, m.Data.Class),
		NextState: SheetExistsState,
	}, nil
}

// takeDamage handler works only in SheetExistsState.
func takeDamage(ctx context.Context, m *corehsm.Machine[CharacterData], cmd *corehsm.Command) (corehsm.Result, error) {
	if len(cmd.Args()) != 1 {
		return corehsm.Result{}, fmt.Errorf("usage: take-damage [amount]")
	}
	amount, err := strconv.Atoi(cmd.Args()[0])
	if err != nil {
		return corehsm.Result{}, fmt.Errorf("invalid amount: must be a number")
	}
	m.Data.HP -= amount
	if m.Data.HP < 0 {
		m.Data.HP = 0
	}
	return corehsm.Result{Output: fmt.Sprintf("%s takes %d damage!", m.Data.Name, amount)}, nil
}

// levelup handler works only in SheetExistsState.
func levelup(ctx context.Context, m *corehsm.Machine[CharacterData], cmd *corehsm.Command) (corehsm.Result, error) {
	m.Data.Level++
	// Increase MaxHP by Constitution bonus on level up
	conBonus := int(math.Floor(float64(m.Data.BaseStats.CON-10) / 2.0))
	hpIncrease := 5 + conBonus
	if hpIncrease < 1 {
		hpIncrease = 1
	}
	m.Data.MaxHP += hpIncrease
	m.Data.HP = m.Data.MaxHP // Restore full HP on level up
	return corehsm.Result{Output: fmt.Sprintf("%s reached level %d!", m.Data.Name, m.Data.Level)}, nil
}

// --- 4. Main Orchestrator ---
func main() {
	registry := corehsm.NewRegistry[CharacterData]()

	// Register States
	registry.RegisterState(NoSheetState)
	registry.RegisterState(SheetExistsState)

	// Register Commands
	registry.RegisterCommand(NoSheetState, corehsm.CommandDef{
		Name: "create", Args: "[name] [race] [class]", Description: "Create a new character.",
	}, create)
	registry.RegisterCommand(SheetExistsState, corehsm.CommandDef{
		Name: "take-damage", Args: "[amount]", Description: "Inflict damage to the character.",
	}, takeDamage)
	registry.RegisterCommand(SheetExistsState, corehsm.CommandDef{
		Name: "levelup", Description: "Level up the character.",
	}, levelup)

	// --- Machine Loading ---
	var m *corehsm.Machine[CharacterData]
	var err error
	snapshot, err := loadSnapshot()
	if err != nil {
		// Start in NoSheetState if no snapshot exists
		m, _ = corehsm.NewMachine(registry, NoSheetState, CharacterData{})
	} else {
		m, _ = corehsm.NewMachineFromSnapshot(registry, snapshot)
	}

	// --- Command Execution ---
	if len(os.Args) > 1 {
		cmd := corehsm.NewCommand(os.Args[1], os.Args[2:]...)
		output, err := m.Execute(context.Background(), cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		if output != "" {
			fmt.Println(">", output)
		}
	}

	// --- State Saving & Display ---
	if err := saveSnapshot(m.GetSnapshot()); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving snapshot: %v\n", err)
		os.Exit(1)
	}

	// Always display the current state and available commands at the end
	displayCurrentState(m)
	displayAvailableCommands(m)
}

// --- 5. View Functions ---

// displayCurrentState selects and shows the appropriate 'view' for the current state.
func displayCurrentState(m *corehsm.Machine[CharacterData]) {
	fmt.Println() // Add a newline for spacing
	if m.CurrentState == NoSheetState {
		fmt.Println("No character sheet found. Create one to begin.")
	} else {
		displayCharacterSheet(m.Data)
	}
}

// displayCharacterSheet takes CharacterData (state) as input and renders it to the CLI.
func displayCharacterSheet(data CharacterData) {
	bar := strings.Repeat("-", 50)
	fmt.Println(bar)
	fmt.Printf("| %-20s | Race: %-19s |\n", "Name: "+data.Name, data.Race)
	fmt.Printf("| %-20s | Level: %-18d |\n", "Class: "+data.Class, data.Level)
	fmt.Println(bar)
	// HP Bar
	hpPercentage := 0.0
	if data.MaxHP > 0 {
		hpPercentage = float64(data.HP) / float64(data.MaxHP)
	}
	hpBlocks := int(hpPercentage * 20)
	hpBar := fmt.Sprintf("[%s%s]", strings.Repeat("#", hpBlocks), strings.Repeat(" ", 20-hpBlocks))
	fmt.Printf("| HP: %-17s %-22s |\n", fmt.Sprintf("%d / %d", data.HP, data.MaxHP), hpBar)
	// Stats
	stats := fmt.Sprintf("STR: %d, DEX: %d, CON: %d", data.BaseStats.STR, data.BaseStats.DEX, data.BaseStats.CON)
	fmt.Printf("| Stats: %-41s |\n", stats)
	fmt.Println(bar)
	// Inventory
	fmt.Println("| Inventory:                                       |")
	if len(data.Inventory) == 0 {
		fmt.Println("|   - (Empty)                                      |")
	} else {
		for _, item := range data.Inventory {
			fmt.Printf("|   - %-42s |\n", item)
		}
	}
	fmt.Println(bar)
}

// displayAvailableCommands shows the list of commands executable in the current state.
func displayAvailableCommands(m *corehsm.Machine[CharacterData]) {
	fmt.Println("\nAvailable Commands:")
	cmds := m.Registry().FindAvailableCommands(m.CurrentState)
	if len(cmds) == 0 {
		fmt.Println("  (None)")
		return
	}
	for _, cmd := range cmds {
		fmt.Printf("  - %-15s %-20s %s\n", cmd.Name, cmd.Args, cmd.Description)
	}
}

// --- 6. Persistence Helpers ---
func loadSnapshot() (*corehsm.Snapshot[CharacterData], error) {
	if _, err := os.Stat(snapshotPath); os.IsNotExist(err) {
		return nil, err
	}
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, err
	}
	var snapshot corehsm.Snapshot[CharacterData]
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func saveSnapshot(snapshot *corehsm.Snapshot[CharacterData]) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(snapshotPath, data, 0644)
}
