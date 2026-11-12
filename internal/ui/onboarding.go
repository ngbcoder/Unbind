package ui

import (
	"os"

	goui "github.com/golang-gui/goui/ui"
)

// OnboardingState tracks which onboarding steps are complete.
type OnboardingState struct {
	// ShowOnboarding reports whether the onboarding should be shown.
	ShowOnboarding bool

	// FFmpegFound reports whether FFmpeg was located.
	FFmpegFound bool

	// OutputDirSet reports whether the output directory is configured.
	OutputDirSet bool

	// FileAssocDecided reports whether the user decided on file associations.
	FileAssocDecided bool

	// CurrentStep is the index of the current wizard step.
	CurrentStep int
}

// NewOnboardingState returns the onboarding state for a fresh install.
func NewOnboardingState(settingsPath string) *OnboardingState {
	s := &OnboardingState{
		ShowOnboarding: true,
		CurrentStep:    0,
	}

	// If the settings file already exists, this is not a first run.
	if _, err := os.Stat(settingsPath); err == nil {
		s.ShowOnboarding = false
	}

	return s
}

// StepCount returns the number of wizard steps.
func (s *OnboardingState) StepCount() int {
	return 4
}

// StepTitle returns the title of the given step.
func (s *OnboardingState) StepTitle(step int) string {
	titles := []string{
		"Welcome",
		"FFmpeg",
		"Output directory",
		"Getting started",
	}
	if step < 0 || step >= len(titles) {
		return ""
	}
	return titles[step]
}

// StepDescription returns the description of the given step.
func (s *OnboardingState) StepDescription(step int) string {
	descriptions := []string{
		"Unbind converts Audible audiobooks to ordinary audio files. This wizard helps you get started.",
		"Unbind uses FFmpeg to read and convert audio. FFmpeg must be available on your system.",
		"Choose where the converted files will be saved.",
		"You are ready to use Unbind. Drag .aax files into the window or use the Add files button.",
	}
	if step < 0 || step >= len(descriptions) {
		return ""
	}
	return descriptions[step]
}

// IsStepComplete reports whether the given step is complete.
func (s *OnboardingState) IsStepComplete(step int) bool {
	switch step {
	case 0:
		return true // welcome is always complete
	case 1:
		return s.FFmpegFound
	case 2:
		return s.OutputDirSet
	case 3:
		return true // getting started is always complete
	}
	return false
}

// NextStep advances to the next step if the current one is complete. When the
// last step is completed the wizard is marked as finished.
func (s *OnboardingState) NextStep() bool {
	if !s.IsStepComplete(s.CurrentStep) {
		return false
	}
	if s.CurrentStep < s.StepCount()-1 {
		s.CurrentStep++
		return true
	}
	s.ShowOnboarding = false
	return true
}

// PrevStep goes back to the previous step.
func (s *OnboardingState) PrevStep() {
	if s.CurrentStep > 0 {
		s.CurrentStep--
	}
}

// IsComplete reports whether all steps are done.
func (s *OnboardingState) IsComplete() bool {
	return s.CurrentStep == s.StepCount()-1 && s.IsStepComplete(s.CurrentStep)
}

// Skip marks the wizard as complete without finishing.
func (s *OnboardingState) Skip() {
	s.ShowOnboarding = false
}

// Finish marks the wizard as complete.
func (s *OnboardingState) Finish() {
	s.ShowOnboarding = false
}

// OnboardingView builds the onboarding wizard view.
func (s *AppState) OnboardingView(onb *OnboardingState) goui.View {
	return goui.VBox(
		goui.Label(onb.StepTitle(onb.CurrentStep)).Style("title"),
		goui.Label(onb.StepDescription(onb.CurrentStep)).Style("body"),
		goui.HBox(
			goui.Button("Back").OnClick(func() { onb.PrevStep(); s.app.RequestUpdate() }),
			goui.Button("Skip").OnClick(func() { onb.Skip(); s.app.RequestUpdate() }),
			goui.Button("Next").OnClick(func() {
				if onb.NextStep() {
					s.app.RequestUpdate()
				}
			}),
		).Spacing(8),
	).Spacing(16).Padding(24)
}
