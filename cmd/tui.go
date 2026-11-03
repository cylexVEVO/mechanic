package cmd

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"mechanic/service"
	"mechanic/store"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive terminal interface",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		program := tea.NewProgram(newTUIModel(), tea.WithAltScreen())
		_, err := program.Run()
		return err
	},
}

type tuiRow struct {
	service *store.Service
	status  *service.Status
}

type tuiModel struct {
	rows          []tuiRow
	cursor        int
	width         int
	height        int
	system        bool
	loading       bool
	busy          bool
	confirmRemove bool
	message       string
	creating      bool
	createForm    tuiCreateForm
	detailOpen    bool
	logsOpen      bool
	logName       string
	logLines      []string
	logOffset     int
	logLoading    bool
}

type tuiCreateForm struct {
	editing         bool
	field           int
	name            string
	command         string
	description     string
	workingDir      string
	env             string
	originalEnvText string
	originalEnv     []string
	restart         int
	start           bool
	err             string
}

var tuiRestartPolicies = []string{"on-failure", "always", "never"}

type tuiRowsMsg struct {
	rows []tuiRow
	err  error
}

type tuiActionMsg struct {
	action string
	name   string
	err    error
}

type tuiTickMsg time.Time

type tuiLogsMsg struct {
	name  string
	lines []string
	err   error
}

type tuiCreateMsg struct {
	name    string
	created bool
	edited  bool
	err     error
}

var (
	tuiTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	tuiHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	tuiSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	tuiRunningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	tuiFailedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	tuiMutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	tuiMessageStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
)

func newTUIModel() tuiModel {
	return tuiModel{loading: true, system: systemScope}
}

func newTUICreateForm() tuiCreateForm {
	home, _ := os.UserHomeDir()
	return tuiCreateForm{workingDir: home}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(loadTUIRows(m.system), tuiTick())
}

func loadTUIRows(system bool) tea.Cmd {
	return func() tea.Msg {
		st, err := store.NewForScope(system)
		if err != nil {
			return tuiRowsMsg{err: err}
		}
		services := st.List()
		sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })

		rows := make([]tuiRow, 0, len(services))
		for _, svc := range services {
			mgr := service.NewForScope(system)
			status, statusErr := mgr.Status(svc.Name)
			if statusErr != nil {
				status = &service.Status{Name: svc.Name, State: "unknown", Message: statusErr.Error()}
			}
			rows = append(rows, tuiRow{service: svc, status: status})
		}
		return tuiRowsMsg{rows: rows}
	}
}

func tuiTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tuiTickMsg(t) })
}

func loadTUILogs(name string, system bool) tea.Cmd {
	return func() tea.Msg {
		out, err := service.NewForScope(system).ReadLogs(name, 500)
		lines := strings.Split(out, "\n")
		if out == "" {
			lines = nil
		}
		return tuiLogsMsg{name: name, lines: lines, err: err}
	}
}

func runTUICreate(form tuiCreateForm, system bool) tea.Cmd {
	return func() tea.Msg {
		env, err := form.environment()
		if err != nil {
			return tuiCreateMsg{edited: form.editing, err: err}
		}
		if form.editing {
			st, err := store.NewForScope(system)
			if err != nil {
				return tuiCreateMsg{edited: true, err: err}
			}
			svc, err := saveServiceEdit(st, service.NewForScope(system), &store.Service{
				Name: form.name, Command: form.command, Description: form.description,
				WorkingDir: strings.TrimSpace(form.workingDir), Restart: store.RestartPolicy(tuiRestartPolicies[form.restart]), Env: env,
			}, form.start)
			return tuiCreateMsg{name: form.name, edited: true, created: svc != nil, err: err}
		}
		svc, err := createManagedService(createOptions{
			name:        strings.TrimSpace(form.name),
			command:     form.command,
			description: form.description,
			workingDir:  strings.TrimSpace(form.workingDir),
			restart:     tuiRestartPolicies[form.restart],
			env:         env,
			start:       form.start,
			system:      system,
		})
		return tuiCreateMsg{name: strings.TrimSpace(form.name), created: svc != nil, err: err}
	}
}

func runTUIAction(action, name string, system bool) tea.Cmd {
	return func() tea.Msg {
		mgr := service.NewForScope(system)
		var err error

		switch action {
		case "start":
			err = mgr.Start(name)
		case "stop":
			err = mgr.Stop(name)
		case "restart":
			err = mgr.Restart(name)
		case "enable", "disable":
			st, storeErr := store.NewForScope(system)
			if storeErr != nil {
				err = storeErr
				break
			}
			svc, getErr := st.Get(name)
			if getErr != nil {
				err = getErr
				break
			}
			if action == "enable" {
				err = mgr.Enable(name)
				svc.Enabled = err == nil
			} else {
				err = mgr.Disable(name)
				if err == nil {
					svc.Enabled = false
				}
			}
			if err == nil {
				err = st.Save()
			}
		case "remove":
			_ = mgr.Stop(name)
			if err = mgr.Uninstall(name); err != nil {
				break
			}
			st, storeErr := store.NewForScope(system)
			if storeErr != nil {
				err = storeErr
				break
			}
			if err = st.Remove(name); err == nil {
				err = st.Save()
			}
		}

		return tuiActionMsg{action: action, name: name, err: err}
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tuiRowsMsg:
		m.loading = false
		if msg.err != nil {
			m.message = "Refresh failed: " + msg.err.Error()
			return m, nil
		}
		m.rows = msg.rows
		if len(m.rows) == 0 {
			m.cursor = 0
		} else if m.cursor >= len(m.rows) {
			m.cursor = len(m.rows) - 1
		}

	case tuiActionMsg:
		m.busy = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Could not %s %s: %v", msg.action, msg.name, msg.err)
		} else {
			m.message = fmt.Sprintf("%s: %s", titleAction(msg.action), msg.name)
		}
		return m, loadTUIRows(m.system)

	case tuiCreateMsg:
		m.busy = false
		if msg.err != nil {
			if msg.created {
				m.creating = false
				m.createForm = tuiCreateForm{}
				m.message = msg.err.Error()
				return m, loadTUIRows(m.system)
			}
			m.creating = true
			m.createForm.err = msg.err.Error()
			return m, nil
		}
		m.creating = false
		m.createForm = tuiCreateForm{}
		m.message = "Created: " + msg.name
		if msg.edited {
			m.message = "Updated: " + msg.name + ". Restart to apply changes if not restarted now."
		}
		return m, loadTUIRows(m.system)

	case tuiLogsMsg:
		m.logLoading = false
		oldMax := maxInt(0, len(m.logLines)-m.logViewportHeight())
		wasAtBottom := m.logOffset >= oldMax
		if msg.err != nil {
			m.message = "Could not read logs: " + msg.err.Error()
			return m, nil
		}
		m.message = ""
		m.logLines = msg.lines
		newMax := maxInt(0, len(m.logLines)-m.logViewportHeight())
		if wasAtBottom || m.logOffset > newMax {
			m.logOffset = newMax
		}

	case tuiTickMsg:
		if m.creating {
			return m, tuiTick()
		}
		if m.logsOpen {
			m.logLoading = true
			return m, tea.Batch(loadTUILogs(m.logName, m.system), tuiTick())
		}
		if !m.busy && !m.loading {
			m.loading = true
			return m, tea.Batch(loadTUIRows(m.system), tuiTick())
		}
		return m, tuiTick()

	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.creating {
			if m.busy {
				return m, nil
			}
			submit, cancel := updateTUICreateForm(&m.createForm, msg)
			if cancel {
				m.creating = false
				m.message = "Creation cancelled"
				if m.createForm.editing {
					m.message = "Editing cancelled"
				}
				return m, nil
			}
			if submit {
				m.busy = true
				m.createForm.err = "Creating service…"
				if m.createForm.editing {
					m.createForm.err = "Saving service…"
				}
				return m, runTUICreate(m.createForm, m.system)
			}
			return m, nil
		}
		if m.logsOpen {
			switch key {
			case "esc", "q":
				m.logsOpen = false
				m.message = ""
			case "up", "k":
				m.logOffset = maxInt(0, m.logOffset-1)
			case "down", "j":
				m.logOffset = minInt(maxInt(0, len(m.logLines)-m.logViewportHeight()), m.logOffset+1)
			case "pgup", "ctrl+b":
				m.logOffset = maxInt(0, m.logOffset-m.logViewportHeight())
			case "pgdown", "ctrl+f":
				m.logOffset = minInt(maxInt(0, len(m.logLines)-m.logViewportHeight()), m.logOffset+m.logViewportHeight())
			case "g", "home":
				m.logOffset = 0
			case "G", "end":
				m.logOffset = maxInt(0, len(m.logLines)-m.logViewportHeight())
			case "r":
				m.logLoading = true
				return m, loadTUILogs(m.logName, m.system)
			}
			return m, nil
		}
		if m.detailOpen {
			switch key {
			case "esc", "q", "enter":
				m.detailOpen = false
			case "E":
				if !m.busy && len(m.rows) > 0 {
					m.openEdit()
				}
			case "l":
				if len(m.rows) > 0 {
					m.detailOpen = false
					m.openLogs()
					return m, loadTUILogs(m.logName, m.system)
				}
			}
			return m, nil
		}
		if m.confirmRemove {
			switch key {
			case "y", "Y":
				m.confirmRemove = false
				if len(m.rows) > 0 {
					name := m.rows[m.cursor].service.Name
					m.busy = true
					m.message = "Removing " + name + "…"
					return m, runTUIAction("remove", name, m.system)
				}
			case "n", "N", "esc":
				m.confirmRemove = false
				m.message = "Removal cancelled"
			}
			return m, nil
		}

		switch key {
		case "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case "r":
			if !m.busy {
				m.loading = true
				m.message = "Refreshing…"
				return m, loadTUIRows(m.system)
			}
		case "E":
			if !m.busy && len(m.rows) > 0 {
				m.openEdit()
			}
		case "c":
			if m.busy {
				return m, nil
			}
			m.creating = true
			m.createForm = newTUICreateForm()
			m.message = ""
		case "enter":
			if len(m.rows) > 0 {
				m.detailOpen = true
			}
		case "l":
			if len(m.rows) > 0 {
				m.openLogs()
				return m, loadTUILogs(m.logName, m.system)
			}
		case "s", "x", "R", "e", "d":
			if m.busy || len(m.rows) == 0 {
				return m, nil
			}
			row := m.rows[m.cursor]
			if key == "d" {
				m.confirmRemove = true
				m.message = fmt.Sprintf("Remove %s? (y/N)", row.service.Name)
				return m, nil
			}
			action := map[string]string{"s": "start", "x": "stop", "R": "restart"}[key]
			if key == "e" {
				action = "enable"
				if row.service.Enabled {
					action = "disable"
				}
			}
			m.busy = true
			m.message = fmt.Sprintf("%s %s…", titleAction(action), row.service.Name)
			return m, runTUIAction(action, row.service.Name, m.system)
		}
	}

	return m, nil
}

func (m *tuiModel) openLogs() {
	m.logsOpen = true
	m.logName = m.rows[m.cursor].service.Name
	m.logLines = nil
	m.logOffset = 0
	m.logLoading = true
	m.message = ""
}

func updateTUICreateForm(form *tuiCreateForm, msg tea.KeyMsg) (submit, cancel bool) {
	key := msg.String()
	form.err = ""
	switch key {
	case "esc":
		return false, true
	case "ctrl+s":
		return true, false
	case "tab", "down":
		form.field = (form.field + 1) % 7
		if form.editing && form.field == 0 {
			form.field = 1
		}
		return false, false
	case "shift+tab", "up":
		form.field = (form.field + 6) % 7
		if form.editing && form.field == 0 {
			form.field = 6
		}
		return false, false
	case "enter":
		if form.field == 6 {
			return true, false
		}
		form.field++
		return false, false
	}

	if form.field == 5 {
		switch key {
		case "left":
			form.restart = (form.restart + len(tuiRestartPolicies) - 1) % len(tuiRestartPolicies)
		case "right", " ":
			form.restart = (form.restart + 1) % len(tuiRestartPolicies)
		}
		return false, false
	}
	if form.field == 6 {
		if key == "left" || key == "right" || key == " " {
			form.start = !form.start
		}
		return false, false
	}

	if key == "backspace" || key == "ctrl+h" {
		form.setCurrent(trimLastRune(form.current()))
		return false, false
	}
	if key == " " {
		form.setCurrent(form.current() + " ")
		return false, false
	}
	if msg.Type == tea.KeyRunes {
		form.setCurrent(form.current() + string(msg.Runes))
	}
	return false, false
}

func (f *tuiCreateForm) current() string {
	switch f.field {
	case 0:
		return f.name
	case 1:
		return f.command
	case 2:
		return f.description
	case 3:
		return f.workingDir
	case 4:
		return f.env
	default:
		return ""
	}
}

func (f *tuiCreateForm) setCurrent(value string) {
	switch f.field {
	case 0:
		if f.editing {
			return
		}
		f.name = value
	case 1:
		f.command = value
	case 2:
		f.description = value
	case 3:
		f.workingDir = value
	case 4:
		f.env = value
	}
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	return string(runes[:len(runes)-1])
}

func (m tuiModel) logViewportHeight() int {
	return maxInt(1, m.height-7)
}

func (m tuiModel) View() string {
	if m.creating {
		return m.createView()
	}
	if m.logsOpen {
		return m.logsView()
	}
	if m.detailOpen {
		return m.detailView()
	}

	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render("mechanic"))
	b.WriteString(tuiMutedStyle.Render("  service dashboard"))
	b.WriteString("\n\n")

	if len(m.rows) == 0 {
		if m.loading {
			b.WriteString("Loading services…\n")
		} else {
			b.WriteString("No services found. Press c to create one.\n")
		}
	} else {
		b.WriteString(tuiHeaderStyle.Render(fmt.Sprintf("  %-20s %-10s %-8s %-8s %s", "NAME", "STATE", "SCOPE", "ENABLED", "COMMAND")))
		b.WriteString("\n")

		available := m.height - 9
		if available < 1 {
			available = 1
		}
		start := 0
		if m.cursor >= available {
			start = m.cursor - available + 1
		}
		end := start + available
		if end > len(m.rows) {
			end = len(m.rows)
		}

		commandWidth := m.width - 56
		if commandWidth < 10 {
			commandWidth = 10
		}
		for index := start; index < end; index++ {
			row := m.rows[index]
			marker := " "
			if index == m.cursor {
				marker = ">"
			}
			enabled := "no"
			if row.service.Enabled {
				enabled = "yes"
			}
			scope := "user"
			if m.system {
				scope = "system"
			}
			state := row.status.State
			stateRendered := fmt.Sprintf("%-10s", state)
			switch state {
			case "running":
				stateRendered = tuiRunningStyle.Render(stateRendered)
			case "failed":
				stateRendered = tuiFailedStyle.Render(stateRendered)
			}
			line := fmt.Sprintf("%s %-20s %s %-8s %-8s %s", marker, row.service.Name, stateRendered, scope, enabled,
				truncateTUI(row.service.Command, commandWidth))
			if index == m.cursor {
				line = tuiSelectedStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	if m.message != "" {
		b.WriteString(tuiMessageStyle.Render(m.message))
		b.WriteString("\n")
	}
	b.WriteString(tuiMutedStyle.Render("↑/k ↓/j move  c create  E edit  enter status  l logs  s start  x stop  R restart  e enable  d remove  r refresh  q quit"))
	return b.String()
}

func (m tuiModel) createView() string {
	var b strings.Builder
	scope := "user"
	if m.system {
		scope = "system"
	}
	action := "Create"
	if m.createForm.editing {
		action = "Edit"
	}
	b.WriteString(tuiTitleStyle.Render(action + " " + scope + " service"))
	b.WriteString("\n\n")

	values := []string{
		m.createForm.name,
		m.createForm.command,
		m.createForm.description,
		m.createForm.workingDir,
		m.createForm.env,
		tuiRestartPolicies[m.createForm.restart],
		map[bool]string{true: "yes", false: "no"}[m.createForm.start],
	}
	labels := []string{"Name", "Command", "Description", "Working dir", "Environment", "Restart", "Start now"}
	if m.createForm.editing {
		labels[0] = "Name (fixed)"
		labels[6] = "Restart now"
	}
	for index, label := range labels {
		marker := " "
		value := values[index]
		if index == m.createForm.field {
			marker = ">"
			if index < 5 {
				value += "▏"
			}
		}
		line := fmt.Sprintf("%s %-14s %s", marker, label+":", value)
		if index == m.createForm.field {
			line = tuiSelectedStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(tuiMutedStyle.Render("Environment: comma-separated KEY=VALUE entries; quote entries containing commas."))
	b.WriteString("\n\n")
	if m.createForm.err != "" {
		b.WriteString(tuiMessageStyle.Render(m.createForm.err))
		b.WriteString("\n")
	}
	b.WriteString(tuiMutedStyle.Render("tab/↑/↓ fields  ←/→ choose  ctrl+s save  enter advances/saves  esc cancel"))
	return b.String()
}

func (m tuiModel) detailView() string {
	if len(m.rows) == 0 {
		return "No service selected.\n"
	}
	row := m.rows[m.cursor]
	svc := row.service
	status := row.status
	scope := "user"
	if m.system {
		scope = "system"
	}
	pid := "-"
	if status.PID > 0 {
		pid = fmt.Sprintf("%d", status.PID)
	}
	since := status.Since
	if since == "" {
		since = "-"
	}
	description := svc.Description
	if description == "" {
		description = "-"
	}
	enabled := "no"
	if svc.Enabled {
		enabled = "yes"
	}

	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render("Service: " + svc.Name))
	b.WriteString("\n\n")
	for _, item := range [][2]string{
		{"State", status.State}, {"PID", pid}, {"Since", since}, {"Scope", scope},
		{"Enabled", enabled}, {"Restart", string(svc.Restart)}, {"Description", description},
		{"Command", svc.Command}, {"Working dir", svc.WorkingDir},
	} {
		b.WriteString(fmt.Sprintf("%-14s %s\n", item[0]+":", item[1]))
	}
	if len(svc.Env) > 0 {
		b.WriteString(fmt.Sprintf("%-14s %s\n", "Environment:", strings.Join(svc.Env, ", ")))
	}
	if status.Message != "" {
		b.WriteString(fmt.Sprintf("%-14s %s\n", "Message:", status.Message))
	}
	b.WriteString("\n")
	b.WriteString(tuiMutedStyle.Render("E edit  l logs  enter/esc/q back"))
	return b.String()
}

func (m tuiModel) logsView() string {
	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render("Logs: " + m.logName))
	if m.logLoading {
		b.WriteString(tuiMutedStyle.Render("  refreshing…"))
	}
	b.WriteString("\n\n")
	height := m.logViewportHeight()
	end := minInt(len(m.logLines), m.logOffset+height)
	if m.message != "" {
		b.WriteString(tuiMessageStyle.Render(truncateTUI(m.message, maxInt(10, m.width-1))))
		b.WriteString("\n")
	} else if len(m.logLines) == 0 {
		b.WriteString(tuiMutedStyle.Render("No log output."))
		b.WriteString("\n")
	} else {
		width := maxInt(10, m.width-1)
		for _, line := range m.logLines[m.logOffset:end] {
			b.WriteString(truncateTUI(line, width))
			b.WriteString("\n")
		}
	}
	for line := end - m.logOffset; line < height; line++ {
		b.WriteString("\n")
	}
	b.WriteString(tuiMutedStyle.Render(fmt.Sprintf("lines %d-%d of %d  ↑/↓ scroll  pgup/pgdn page  g/G ends  r refresh  esc/q back",
		minInt(len(m.logLines), m.logOffset+1), end, len(m.logLines))))
	return b.String()
}

func truncateTUI(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func titleAction(action string) string {
	if action == "" {
		return ""
	}
	return strings.ToUpper(action[:1]) + action[1:]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *tuiModel) openEdit() {
	svc := m.rows[m.cursor].service
	var env bytes.Buffer
	writer := csv.NewWriter(&env)
	if len(svc.Env) > 0 {
		_ = writer.Write(svc.Env)
		writer.Flush()
	}
	form := tuiCreateForm{editing: true, field: 1, name: svc.Name, command: svc.Command,
		description: svc.Description, workingDir: svc.WorkingDir, env: strings.TrimSuffix(env.String(), "\n")}
	form.originalEnvText = form.env
	form.originalEnv = append([]string(nil), svc.Env...)
	for i, policy := range tuiRestartPolicies {
		if policy == string(svc.Restart) {
			form.restart = i
		}
	}
	m.createForm = form
	m.creating, m.detailOpen = true, false
	m.message = ""
}

// Keep untouched environment values byte-for-byte: CSV parsing normalizes CRLF.
func (f tuiCreateForm) environment() ([]string, error) {
	if f.editing && f.env == f.originalEnvText {
		return append([]string(nil), f.originalEnv...), nil
	}
	if f.env == "" {
		return nil, nil
	}
	reader := csv.NewReader(strings.NewReader(f.env))
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil || len(records) != 1 {
		return nil, fmt.Errorf("environment must be one comma-separated record; quote values containing commas")
	}
	return records[0], nil
}
