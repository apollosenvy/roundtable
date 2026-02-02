package commands

import (
	"strings"
	"testing"
)

func TestParse_NonSlashCommand(t *testing.T) {
	tests := []string{
		"hello world",
		"",
		"   ",
		"help",
		"new debate",
		"this is not a command",
	}

	for _, input := range tests {
		result := Parse(input)
		if result != nil {
			t.Errorf("Parse(%q) = %v, want nil", input, result)
		}
	}
}

func TestParse_Help(t *testing.T) {
	tests := []string{
		"/help",
		"/HELP",
		"/Help",
		"  /help  ",
		"/help extra args ignored",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want Help{}", input)
			continue
		}
		if _, ok := result.(Help); !ok {
			t.Errorf("Parse(%q) = %T, want Help", input, result)
		}
		if result.Type() != "help" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "help")
		}
	}
}

func TestParse_NewDebate(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
	}{
		{"/new", ""},
		{"/new My Debate", "My Debate"},
		{"/NEW test", "test"},
		{"  /new  trimmed  ", "trimmed"},
		{"/new one two three", "one two three"},
	}

	for _, tt := range tests {
		result := Parse(tt.input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want NewDebate", tt.input)
			continue
		}
		nd, ok := result.(NewDebate)
		if !ok {
			t.Errorf("Parse(%q) = %T, want NewDebate", tt.input, result)
			continue
		}
		if nd.Name != tt.wantName {
			t.Errorf("Parse(%q).Name = %q, want %q", tt.input, nd.Name, tt.wantName)
		}
		if nd.Type() != "new" {
			t.Errorf("Parse(%q).Type() = %q, want %q", tt.input, nd.Type(), "new")
		}
	}
}

func TestParse_CloseDebate(t *testing.T) {
	tests := []string{
		"/close",
		"/CLOSE",
		"  /close  ",
		"/close extra ignored",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want CloseDebate{}", input)
			continue
		}
		if _, ok := result.(CloseDebate); !ok {
			t.Errorf("Parse(%q) = %T, want CloseDebate", input, result)
		}
		if result.Type() != "close" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "close")
		}
	}
}

func TestParse_RenameDebate(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
	}{
		{"/rename New Name", "New Name"},
		{"/RENAME test", "test"},
		{"/rename one two three", "one two three"},
	}

	for _, tt := range tests {
		result := Parse(tt.input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want RenameDebate", tt.input)
			continue
		}
		rd, ok := result.(RenameDebate)
		if !ok {
			t.Errorf("Parse(%q) = %T, want RenameDebate", tt.input, result)
			continue
		}
		if rd.Name != tt.wantName {
			t.Errorf("Parse(%q).Name = %q, want %q", tt.input, rd.Name, tt.wantName)
		}
		if rd.Type() != "rename" {
			t.Errorf("Parse(%q).Type() = %q, want %q", tt.input, rd.Type(), "rename")
		}
	}
}

func TestParse_RenameDebate_NoName(t *testing.T) {
	tests := []string{
		"/rename",
		"/rename   ",
		"  /rename  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ParseError", input)
			continue
		}
		pe, ok := result.(ParseError)
		if !ok {
			t.Errorf("Parse(%q) = %T, want ParseError", input, result)
			continue
		}
		if !strings.Contains(pe.Message, "requires a name") {
			t.Errorf("Parse(%q).Message = %q, want message containing 'requires a name'", input, pe.Message)
		}
		if pe.Type() != "error" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, pe.Type(), "error")
		}
	}
}

func TestParse_ContextAdd(t *testing.T) {
	tests := []struct {
		input    string
		wantPath string
	}{
		{"/context add /path/to/file", "/path/to/file"},
		{"/CONTEXT ADD ./relative/path", "./relative/path"},
		{"/context add path with spaces", "path with spaces"},
	}

	for _, tt := range tests {
		result := Parse(tt.input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want AddContext", tt.input)
			continue
		}
		ac, ok := result.(AddContext)
		if !ok {
			t.Errorf("Parse(%q) = %T, want AddContext", tt.input, result)
			continue
		}
		if ac.Path != tt.wantPath {
			t.Errorf("Parse(%q).Path = %q, want %q", tt.input, ac.Path, tt.wantPath)
		}
		if ac.Type() != "context_add" {
			t.Errorf("Parse(%q).Type() = %q, want %q", tt.input, ac.Type(), "context_add")
		}
	}
}

func TestParse_ContextAdd_NoPath(t *testing.T) {
	tests := []string{
		"/context add",
		"/context add   ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ParseError", input)
			continue
		}
		pe, ok := result.(ParseError)
		if !ok {
			t.Errorf("Parse(%q) = %T, want ParseError", input, result)
			continue
		}
		if !strings.Contains(pe.Message, "requires a path") {
			t.Errorf("Parse(%q).Message = %q, want message containing 'requires a path'", input, pe.Message)
		}
	}
}

func TestParse_ContextRemove(t *testing.T) {
	tests := []struct {
		input    string
		wantPath string
	}{
		{"/context remove /path/to/file", "/path/to/file"},
		{"/CONTEXT REMOVE ./relative/path", "./relative/path"},
		{"/context remove path with spaces", "path with spaces"},
	}

	for _, tt := range tests {
		result := Parse(tt.input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want RemoveContext", tt.input)
			continue
		}
		rc, ok := result.(RemoveContext)
		if !ok {
			t.Errorf("Parse(%q) = %T, want RemoveContext", tt.input, result)
			continue
		}
		if rc.Path != tt.wantPath {
			t.Errorf("Parse(%q).Path = %q, want %q", tt.input, rc.Path, tt.wantPath)
		}
		if rc.Type() != "context_remove" {
			t.Errorf("Parse(%q).Type() = %q, want %q", tt.input, rc.Type(), "context_remove")
		}
	}
}

func TestParse_ContextRemove_NoPath(t *testing.T) {
	tests := []string{
		"/context remove",
		"/context remove   ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ParseError", input)
			continue
		}
		pe, ok := result.(ParseError)
		if !ok {
			t.Errorf("Parse(%q) = %T, want ParseError", input, result)
			continue
		}
		if !strings.Contains(pe.Message, "requires a path") {
			t.Errorf("Parse(%q).Message = %q, want message containing 'requires a path'", input, pe.Message)
		}
	}
}

func TestParse_ContextList(t *testing.T) {
	tests := []string{
		"/context list",
		"/CONTEXT LIST",
		"/context list extra ignored",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ListContext{}", input)
			continue
		}
		if _, ok := result.(ListContext); !ok {
			t.Errorf("Parse(%q) = %T, want ListContext", input, result)
		}
		if result.Type() != "context_list" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "context_list")
		}
	}
}

func TestParse_ContextNoSubcommand(t *testing.T) {
	tests := []string{
		"/context",
		"/context   ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ParseError", input)
			continue
		}
		pe, ok := result.(ParseError)
		if !ok {
			t.Errorf("Parse(%q) = %T, want ParseError", input, result)
			continue
		}
		if !strings.Contains(pe.Message, "requires a subcommand") {
			t.Errorf("Parse(%q).Message = %q, want message containing 'requires a subcommand'", input, pe.Message)
		}
	}
}

func TestParse_ContextUnknownSubcommand(t *testing.T) {
	tests := []string{
		"/context foo",
		"/context unknown",
		"/context clear",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ParseError", input)
			continue
		}
		pe, ok := result.(ParseError)
		if !ok {
			t.Errorf("Parse(%q) = %T, want ParseError", input, result)
			continue
		}
		if !strings.Contains(pe.Message, "unknown context subcommand") {
			t.Errorf("Parse(%q).Message = %q, want message containing 'unknown context subcommand'", input, pe.Message)
		}
	}
}

func TestParse_Models(t *testing.T) {
	tests := []string{
		"/models",
		"/MODELS",
		"  /models  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ToggleModels{}", input)
			continue
		}
		if _, ok := result.(ToggleModels); !ok {
			t.Errorf("Parse(%q) = %T, want ToggleModels", input, result)
		}
		if result.Type() != "models" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "models")
		}
	}
}

func TestParse_Consensus(t *testing.T) {
	tests := []string{
		"/consensus",
		"/CONSENSUS",
		"  /consensus  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ForceConsensus{}", input)
			continue
		}
		if _, ok := result.(ForceConsensus); !ok {
			t.Errorf("Parse(%q) = %T, want ForceConsensus", input, result)
		}
		if result.Type() != "consensus" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "consensus")
		}
	}
}

func TestParse_Execute(t *testing.T) {
	tests := []string{
		"/execute",
		"/EXECUTE",
		"  /execute  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want Execute{}", input)
			continue
		}
		if _, ok := result.(Execute); !ok {
			t.Errorf("Parse(%q) = %T, want Execute", input, result)
		}
		if result.Type() != "execute" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "execute")
		}
	}
}

func TestParse_Pause(t *testing.T) {
	tests := []string{
		"/pause",
		"/PAUSE",
		"  /pause  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want Pause{}", input)
			continue
		}
		if _, ok := result.(Pause); !ok {
			t.Errorf("Parse(%q) = %T, want Pause", input, result)
		}
		if result.Type() != "pause" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "pause")
		}
	}
}

func TestParse_Resume(t *testing.T) {
	tests := []string{
		"/resume",
		"/RESUME",
		"  /resume  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want Resume{}", input)
			continue
		}
		if _, ok := result.(Resume); !ok {
			t.Errorf("Parse(%q) = %T, want Resume", input, result)
		}
		if result.Type() != "resume" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "resume")
		}
	}
}

func TestParse_History(t *testing.T) {
	tests := []string{
		"/history",
		"/HISTORY",
		"  /history  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ShowHistory{}", input)
			continue
		}
		if _, ok := result.(ShowHistory); !ok {
			t.Errorf("Parse(%q) = %T, want ShowHistory", input, result)
		}
		if result.Type() != "history" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "history")
		}
	}
}

func TestParse_Export(t *testing.T) {
	tests := []string{
		"/export",
		"/EXPORT",
		"  /export  ",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want Export{}", input)
			continue
		}
		if _, ok := result.(Export); !ok {
			t.Errorf("Parse(%q) = %T, want Export", input, result)
		}
		if result.Type() != "export" {
			t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), "export")
		}
	}
}

func TestParse_UnknownCommand(t *testing.T) {
	tests := []string{
		"/unknown",
		"/foo",
		"/bar baz",
		"/quit",
		"/exit",
	}

	for _, input := range tests {
		result := Parse(input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want ParseError", input)
			continue
		}
		pe, ok := result.(ParseError)
		if !ok {
			t.Errorf("Parse(%q) = %T, want ParseError", input, result)
			continue
		}
		if !strings.Contains(pe.Message, "unknown command") {
			t.Errorf("Parse(%q).Message = %q, want message containing 'unknown command'", input, pe.Message)
		}
	}
}

func TestParse_SlashOnly(t *testing.T) {
	// A lone "/" is an invalid command, should return ParseError
	result := Parse("/")
	if result == nil {
		t.Error("Parse('/') = nil, want ParseError")
		return
	}
	pe, ok := result.(ParseError)
	if !ok {
		t.Errorf("Parse('/') = %T, want ParseError", result)
		return
	}
	if !strings.Contains(pe.Message, "unknown command") {
		t.Errorf("Parse('/').Message = %q, want message containing 'unknown command'", pe.Message)
	}
}

func TestHelpText(t *testing.T) {
	help := HelpText()

	if help == "" {
		t.Error("HelpText() returned empty string")
	}

	// Verify all commands are documented
	expectedCommands := []string{
		"/help",
		"/new",
		"/close",
		"/rename",
		"/context add",
		"/context remove",
		"/context list",
		"/models",
		"/consensus",
		"/execute",
		"/pause",
		"/resume",
		"/history",
		"/export",
	}

	for _, cmd := range expectedCommands {
		if !strings.Contains(help, cmd) {
			t.Errorf("HelpText() missing documentation for %q", cmd)
		}
	}
}

func TestCommandTypes(t *testing.T) {
	// Verify all command types return the expected string
	tests := []struct {
		cmd      Command
		wantType string
	}{
		{Help{}, "help"},
		{NewDebate{}, "new"},
		{CloseDebate{}, "close"},
		{RenameDebate{}, "rename"},
		{AddContext{}, "context_add"},
		{RemoveContext{}, "context_remove"},
		{ListContext{}, "context_list"},
		{ToggleModels{}, "models"},
		{ForceConsensus{}, "consensus"},
		{Execute{}, "execute"},
		{Pause{}, "pause"},
		{Resume{}, "resume"},
		{ShowHistory{}, "history"},
		{Export{}, "export"},
		{ParseError{}, "error"},
	}

	for _, tt := range tests {
		if got := tt.cmd.Type(); got != tt.wantType {
			t.Errorf("%T.Type() = %q, want %q", tt.cmd, got, tt.wantType)
		}
	}
}

func TestParse_CaseInsensitive(t *testing.T) {
	// Verify that commands are case-insensitive
	testCases := []struct {
		inputs  []string
		cmdType string
	}{
		{[]string{"/help", "/HELP", "/Help", "/hElP"}, "help"},
		{[]string{"/new", "/NEW", "/New"}, "new"},
		{[]string{"/close", "/CLOSE", "/Close"}, "close"},
		{[]string{"/models", "/MODELS", "/Models"}, "models"},
	}

	for _, tc := range testCases {
		for _, input := range tc.inputs {
			result := Parse(input)
			if result == nil {
				t.Errorf("Parse(%q) = nil, want command of type %q", input, tc.cmdType)
				continue
			}
			if result.Type() != tc.cmdType {
				t.Errorf("Parse(%q).Type() = %q, want %q", input, result.Type(), tc.cmdType)
			}
		}
	}
}

func TestParse_WhitespaceHandling(t *testing.T) {
	// Verify proper whitespace handling
	tests := []struct {
		input string
		want  string // expected type
	}{
		{"   /help   ", "help"},
		{"\t/close\t", "close"},
		{"/new   ", "new"},
		{"/context   add   /path", "context_add"},
	}

	for _, tt := range tests {
		result := Parse(tt.input)
		if result == nil {
			t.Errorf("Parse(%q) = nil, want command of type %q", tt.input, tt.want)
			continue
		}
		if result.Type() != tt.want {
			t.Errorf("Parse(%q).Type() = %q, want %q", tt.input, result.Type(), tt.want)
		}
	}
}
