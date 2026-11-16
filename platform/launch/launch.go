package launch

import (
	"os"
	"os/exec"
	"runtime"
)

// AppWindow opens targetURL in a standalone WebView/Chrome/Edge app window when available,
// falling back to the OS default web browser.
func AppWindow(targetURL string) {
	switch runtime.GOOS {
	case "darwin":
		browsers := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
		for _, bin := range browsers {
			if _, err := os.Stat(bin); err == nil {
				if exec.Command(bin, "--app="+targetURL).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("open", targetURL).Start()
	case "linux":
		for _, bin := range []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge"} {
			if p, err := exec.LookPath(bin); err == nil {
				if exec.Command(p, "--app="+targetURL).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("xdg-open", targetURL).Start()
	case "windows":
		for _, bin := range []string{"msedge", "chrome"} {
			if p, err := exec.LookPath(bin); err == nil {
				if exec.Command(p, "--app="+targetURL).Start() == nil {
					return
				}
			}
		}
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
	}
}
