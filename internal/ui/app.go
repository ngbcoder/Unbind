package ui

import (
	"fmt"
	"os"
	"path/filepath"

	goui "github.com/golang-gui/goui/ui"
	wui "github.com/golang-gui/goui/widgets/ui"
)

// AppState holds the state of the application. The fields are owned by the
// application, not by the view, so they survive rebuilds.
type AppState struct {
	app goui.App

	// files are the paths of the files that are loaded.
	files goui.State[[]string]

	// status is the text shown in the status bar.
	status goui.State[string]

	// convertEnabled reports whether the Convert button may be pressed.
	convertEnabled goui.State[bool]

	// onboarding tracks the first-run wizard.
	onboarding *OnboardingState
}

// New returns a new AppState bound to the application.
func New(app goui.App) *AppState {
	s := &AppState{
		app:            app,
		files:          goui.MakeState[[]string](nil),
		status:         goui.MakeState[string]("Drop .aax files here, or use the Add button."),
		convertEnabled: goui.MakeState[bool](false),
		onboarding:     NewOnboardingState(settingsPath(app)),
	}
	return s
}

// App returns the underlying application.
func (s *AppState) App() goui.App { return s.app }

// settingsPath returns the path of the settings file.
func settingsPath(app goui.App) string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "audiamus", "Unbind", "settings.json")
}

// MainView builds the main window content. On a first run the onboarding
// wizard is shown instead of the main view.
func (s *AppState) MainView() goui.View {
	if s.onboarding != nil && s.onboarding.ShowOnboarding {
		return s.OnboardingView(s.onboarding)
	}

	return goui.VBox(
		// Header
		goui.HBox(
			goui.Label("Unbind").Style("title"),
		).Spacing(12),
		// Toolbar
		goui.HBox(
			goui.Button("Add files...").OnClick(func() { s.addFiles() }),
			goui.Button("Convert").OnClick(func() { s.convert() }),
			goui.Button("Abort").OnClick(func() { s.abort() }),
		).Spacing(8),
		// File table placeholder
		goui.Label("File list — drag .aax files here").Style("placeholder"),
		// Progress bars
		goui.HBox(
			goui.Label("Phase:").Style("label"),
			wui.ProgressBar(0).Shape(wui.ProgressLinear).Thickness(4),
		).Spacing(8),
		goui.HBox(
			goui.Label("Step:").Style("label"),
			wui.ProgressBar(0).Shape(wui.ProgressLinear).Thickness(4),
		).Spacing(8),
		// Status bar
		goui.Label(s.status.Get()).Style("status"),
	).Spacing(12).Padding(20)
}

// addFiles opens the native file dialog and adds the selected files.
func (s *AppState) addFiles() {
	opts := goui.DialogOptions{
		Title: "Select Audible files",
		Filters: []goui.FileFilter{
			{Name: "Audible files", Patterns: []string{"*.aax", "*.aa", "*.m4b"}},
			{Name: "All files", Patterns: []string{"*"}},
		},
		AllowMultiple: true,
	}
	s.app.FileDialog().OpenFile(opts, func(access []goui.FileAccess, err error) {
		if err != nil {
			s.status.Set("File dialog failed: " + err.Error())
			return
		}
		if len(access) == 0 {
			return // cancelled
		}
		paths := make([]string, 0, len(access))
		for _, a := range access {
			p, err := a.LocalPath()
			if err != nil {
				continue
			}
			paths = append(paths, p)
			a.Close()
		}
		s.files.Update(func(old []string) []string {
			return append(old, paths...)
		})
		s.status.Set(fmt.Sprintf("Added %d file(s).", len(paths)))
		s.app.RequestUpdate()
	})
}

// convert starts a conversion. A placeholder for now.
func (s *AppState) convert() {
	s.status.Set("Conversion not wired yet (engine in progress).")
}

// abort cancels the running conversion.
func (s *AppState) abort() {
	s.status.Set("Nothing to abort.")
}

// filesTable returns the file table view. A placeholder for now.
func (s *AppState) filesTable() goui.View {
	return goui.Label("File table goes here.")
}
