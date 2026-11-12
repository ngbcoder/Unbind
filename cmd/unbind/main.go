package main

import (
	"log"

	"github.com/golang-gui/goui/theme/modern"
	goui "github.com/golang-gui/goui/ui"

	"github.com/ngbcoder/Unbind/internal/ui"
)

func main() {
	err := goui.Run("com.ngbcoder.unbind", func(app goui.App) goui.RootView {
		state := ui.New(app)
		return goui.Root().
			StyleSheet(modern.FromSettings(app.Settings())).
			Windows(
				goui.Window("main").
					Title("Unbind").
					Size(960, 640).
					Center(true).
					Content(state.MainView()),
			)
	})
	if err != nil {
		log.Fatal(err)
	}
}
