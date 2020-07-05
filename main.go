package main // import "github.com/Jguer/yay"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	alpm "github.com/Jguer/go-alpm"
	pacmanconf "github.com/Morganamilo/go-pacmanconf"
	"github.com/leonelquinteros/gotext"

	"github.com/Jguer/yay/v10/pkg/settings"
	"github.com/Jguer/yay/v10/pkg/text"
)

func initGotext() {
	if envLocalePath := os.Getenv("LOCALE_PATH"); envLocalePath != "" {
		localePath = envLocalePath
	}

	gotext.Configure(localePath, os.Getenv("LANG"), "yay")
}

func initConfig(configPath string) error {
	cfile, err := os.Open(configPath)
	if !os.IsNotExist(err) && err != nil {
		return errors.New(gotext.Get("failed to open config file '%s': %s", configPath, err))
	}

	defer cfile.Close()
	if !os.IsNotExist(err) {
		decoder := json.NewDecoder(cfile)
		if err = decoder.Decode(&config); err != nil {
			return errors.New(gotext.Get("failed to read config file '%s': %s", configPath, err))
		}
	}

	aurdest := os.Getenv("AURDEST")
	if aurdest != "" {
		config.BuildDir = aurdest
	}

	return nil
}

func initVCS(vcsFilePath string) error {
	vfile, err := os.Open(vcsFilePath)
	if !os.IsNotExist(err) && err != nil {
		return errors.New(gotext.Get("failed to open vcs file '%s': %s", vcsFilePath, err))
	}

	defer vfile.Close()
	if !os.IsNotExist(err) {
		decoder := json.NewDecoder(vfile)
		if err = decoder.Decode(&savedInfo); err != nil {
			return errors.New(gotext.Get("failed to read vcs file '%s': %s", vcsFilePath, err))
		}
	}

	return nil
}

func initBuildDir() error {
	if _, err := os.Stat(config.BuildDir); os.IsNotExist(err) {
		if err = os.MkdirAll(config.BuildDir, 0755); err != nil {
			return errors.New(gotext.Get("failed to create BuildDir directory '%s': %s", config.BuildDir, err))
		}
	} else if err != nil {
		return err
	}

	return nil
}

func initAlpm(pacmanConfigPath string) error {
	var err error
	var stderr string

	root := "/"
	if value, _, exists := cmdArgs.GetArg("root", "r"); exists {
		root = value
	}

	pacmanConf, stderr, err = pacmanconf.PacmanConf("--config", pacmanConfigPath, "--root", root)
	if err != nil {
		return fmt.Errorf("%s", stderr)
	}

	if value, _, exists := cmdArgs.GetArg("dbpath", "b"); exists {
		pacmanConf.DBPath = value
	}

	if value, _, exists := cmdArgs.GetArg("arch"); exists {
		pacmanConf.Architecture = value
	}

	if value, _, exists := cmdArgs.GetArg("ignore"); exists {
		pacmanConf.IgnorePkg = append(pacmanConf.IgnorePkg, strings.Split(value, ",")...)
	}

	if value, _, exists := cmdArgs.GetArg("ignoregroup"); exists {
		pacmanConf.IgnoreGroup = append(pacmanConf.IgnoreGroup, strings.Split(value, ",")...)
	}

	// TODO
	// current system does not allow duplicate arguments
	// but pacman allows multiple cachedirs to be passed
	// for now only handle one cache dir
	if value, _, exists := cmdArgs.GetArg("cachedir"); exists {
		pacmanConf.CacheDir = []string{value}
	}

	if value, _, exists := cmdArgs.GetArg("gpgdir"); exists {
		pacmanConf.GPGDir = value
	}

	if err := initAlpmHandle(); err != nil {
		return err
	}

	switch value, _, _ := cmdArgs.GetArg("color"); value {
	case "always":
		text.UseColor = true
	case "auto":
		text.UseColor = isTty()
	case "never":
		text.UseColor = false
	default:
		text.UseColor = pacmanConf.Color && isTty()
	}

	return nil
}

func initAlpmHandle() error {
	if alpmHandle != nil {
		if errRelease := alpmHandle.Release(); errRelease != nil {
			return errRelease
		}
	}

	var err error
	if alpmHandle, err = alpm.Initialize(pacmanConf.RootDir, pacmanConf.DBPath); err != nil {
		return errors.New(gotext.Get("unable to CreateHandle: %s", err))
	}

	if err := configureAlpm(); err != nil {
		return err
	}

	alpmHandle.SetQuestionCallback(questionCallback)
	alpmHandle.SetLogCallback(logCallback)
	return nil
}

func exitOnError(err error) {
	if err != nil {
		if str := err.Error(); str != "" {
			fmt.Fprintln(os.Stderr, str)
		}
		cleanup()
		os.Exit(1)
	}
}

func cleanup() int {
	if alpmHandle != nil {
		if err := alpmHandle.Release(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	return 0
}

func main() {
	initGotext()
	if os.Geteuid() == 0 {
		text.Warnln(gotext.Get("Avoid running yay as root/sudo."))
	}

	runtime, err := settings.MakeRuntime()
	exitOnError(err)
	config = defaultSettings()
	config.Runtime = runtime
	exitOnError(initConfig(runtime.ConfigPath))
	exitOnError(cmdArgs.ParseCommandLine(config))
	if config.Runtime.SaveConfig {
		err := config.SaveConfig(runtime.ConfigPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	config.ExpandEnv()
	exitOnError(initBuildDir())
	exitOnError(initVCS(runtime.VCSPath))
	exitOnError(initAlpm(config.PacmanConf))
	exitOnError(handleCmd())
	os.Exit(cleanup())
}
