package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"

	"github.com/mrpoundsign/Hearsay/assets"
	"github.com/mrpoundsign/Hearsay/internal/config"
	"github.com/mrpoundsign/Hearsay/internal/listener"
	"github.com/mrpoundsign/Hearsay/internal/logging"
	"github.com/mrpoundsign/Hearsay/internal/ui"
	"github.com/mrpoundsign/TownCrier/pkg/audio"
	"github.com/mrpoundsign/TownCrier/pkg/tts"
)

func parseLogFlagEarly() string {
	for i, arg := range os.Args {
		if (arg == "-log" || arg == "--log") && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
		if strings.HasPrefix(arg, "-log=") || strings.HasPrefix(arg, "--log=") {
			parts := strings.SplitN(arg, "=", 2)
			return parts[1]
		}
	}
	return ""
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	logPath, err := logging.Init(parseLogFlagEarly())
	if err != nil {
		log.Printf("Warning: failed to initialize persistent log file: %v", err)
	} else {
		log.Printf("=== Hearsay Started (Log: %s) ===", logPath)
	}
	defer logging.Close()
	defer logging.RecoverAndExit("main")

	flag.CommandLine.SetOutput(log.Writer())
	logFlag := flag.String("log", "", "Path to custom log file")
	tuneFlag := flag.String("tune", "", "Stream URL or platform:channel to tune into on launch")
	flag.Parse()
	_ = logFlag

	if os.Getenv("FYNE_DISABLE_DPI_DETECTION") == "" && os.Getenv("FYNE_SCALE") == "" {
		initialUIScale := ui.UIScale100
		if locs, lErr := config.ProductionLocations(); lErr == nil {
			if preCfg, cErr := config.Load(locs.ConfigFile); cErr == nil && preCfg != nil && preCfg.Accessibility.UIScale != "" {
				initialUIScale = preCfg.Accessibility.UIScale
			}
		}
		ui.ApplyUIScale(nil, initialUIScale, 1.0, false)
	}

	myApp := fyneapp.NewWithID(config.AppID)
	myApp.SetIcon(assets.AppIcon)

	var activeSession *listener.Session
	var activePlayer audio.Player
	var activeEngine tts.Engine

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		sig, ok := <-sigChan
		if !ok {
			return
		}
		log.Printf("Received termination signal (%v), shutting down gracefully...", sig)
		fyne.Do(func() {
			myApp.Quit()
		})
		if sig2, ok := <-sigChan; ok {
			log.Printf("Received second termination signal (%v), forcing exit...", sig2)
			os.Exit(1)
		}
	}()

	tracker, launchWin := ui.NewLaunchWindow(myApp)
	launchWin.SetCloseIntercept(func() {
		log.Println("Launch window closed by user, aborting startup...")
		launchWin.Close()
		myApp.Quit()
	})
	launchWin.Show()

	go func() {
		defer logging.RecoverAndExit("background-init")

		tracker.SetStatus("Loading configuration & saved library...")
		locations, locErr := config.ProductionLocations()
		if locErr != nil {
			log.Fatalf("Fatal: could not resolve configuration locations: %v", locErr)
		}
		configStore, storeErr := config.NewFileStore(locations)
		if storeErr != nil {
			log.Fatalf("Fatal: could not initialize configuration store: %v", storeErr)
		}

		cfg, err := configStore.Load()
		if err != nil {
			log.Printf("Using default config (load error: %v)", err)
			cfg = config.DefaultConfig()
			tracker.SetConfigStatus(false, "Using defaults")
		} else {
			log.Printf("Loaded config.json from %s", configStore.Location())
			tracker.SetConfigStatus(true, "Loaded config.json")
		}

		tracker.SetStatus("Connecting to WASAPI stereo audio subsystem...")
		player, err := audio.NewDevicePlayer(cfg.Audio.OutputDeviceID, 24000)
		if err != nil {
			log.Printf("Warning: failed to open target audio device: %v", err)
			tracker.SetAudioStatus(false, "Using fallback")
		} else {
			activePlayer = player
			if cfg.Audio.MasterVolume > 0 {
				player.SetVolume(float32(cfg.Audio.MasterVolume))
			}
			tracker.SetAudioStatus(true, "WASAPI stereo ready")
		}

		quitApp := func() {
			fyne.Do(func() {
				launchWin.Close()
				myApp.Quit()
			})
		}

		launchMainUI := func(eng *tts.MultiEngine) {
			if eng != nil {
				activeEngine = eng
				tracker.SetTTSStatus(true, eng.Provider())
			} else {
				tracker.SetTTSStatus(false, "Skipped (configure in Settings & Voices)")
			}

			tracker.SetStatus("Ready! Opening Hearsay Radio Deck...")
			time.Sleep(250 * time.Millisecond)

			session := listener.NewSession(cfg, configStore)
			activeSession = session
			if player != nil {
				session.SetPlayer(player)
			}
			if eng != nil {
				session.SetEngine(eng)
			}
			_ = session.Start()

			if strings.TrimSpace(*tuneFlag) != "" {
				if _, tuneErr := session.TuneIn(*tuneFlag, "twitch"); tuneErr != nil {
					log.Printf("Warning: -tune flag error: %v", tuneErr)
				}
			}

			fyne.Do(func() {
				defer logging.RecoverAndExit("ui-init")
				mainWindow := ui.NewMainWindow(myApp, session, audio.EnumeratePlaybackDevices)
				mainWindow.Window.SetMaster()
				mainWindow.Window.Show()
				launchWin.Close()
			})
		}

		var initTTSAndLaunch func()
		skipModelAndContinue := func(failedFam string) func() {
			return func() {
				defer logging.RecoverAndExit("tts-skip")
				modelDir := tts.DefaultModelDir()
				var remaining []string
				for _, m := range cfg.Audio.EffectiveEnabledModels() {
					if m == failedFam {
						continue
					}
					if (failedFam == tts.ModelKokoro || failedFam == tts.ModelKokoroIntl) &&
						(m == tts.ModelKokoro || m == tts.ModelKokoroIntl) {
						continue
					}
					if tts.IsModelFamilyInstalled(modelDir, m, cfg.Audio.UseGPU) {
						remaining = append(remaining, m)
					}
				}
				if len(remaining) == 0 {
					for _, spec := range tts.AllModelFamilies {
						if tts.IsModelFamilyInstalled(modelDir, spec.ID, cfg.Audio.UseGPU) {
							remaining = append(remaining, spec.ID)
							break
						}
					}
				}
				if len(remaining) > 0 {
					cfg.Audio.EnabledModels = remaining
				} else {
					cfg.Audio.EnabledModels = []string{tts.ModelSupertonic}
				}
				_ = configStore.Save(cfg)

				if len(remaining) > 0 {
					threads := cfg.Audio.NumThreads
					if threads <= 0 {
						threads = 4
					}
					if eng, engErr := tts.NewMultiEngine(modelDir, remaining, threads, cfg.Audio.UseGPU, tts.DefaultMaxClauseWords, 0); engErr == nil {
						launchMainUI(eng)
						return
					}
				}
				launchMainUI(nil)
			}
		}

		initTTSAndLaunch = func() {
			defer logging.RecoverAndExit("tts-init")

			modelDir := tts.DefaultModelDir()
			enabledModels := cfg.Audio.EffectiveEnabledModels()

			for _, fam := range enabledModels {
				if fam == tts.ModelKokoro || fam == tts.ModelKokoroIntl {
					if !tts.IsKokoroInstalledForMode(modelDir, cfg.Audio.UseGPU) {
						log.Printf("Kokoro models missing in %s; starting download...", modelDir)
						tracker.SetDownloadModel("Kokoro-82M")
						tracker.SetStatus("Downloading Kokoro-82M neural TTS models...")
						if dlErr := tts.DownloadKokoroModelForMode(modelDir, cfg.Audio.UseGPU, tracker.SetDownloadProgress); dlErr != nil {
							log.Printf("Error: failed to download Kokoro TTS models: %v", dlErr)
							tracker.ShowFatalError(
								"Failed to download Kokoro-82M models. You can retry, skip to launch with installed voices, or quit.",
								"Download failed ("+dlErr.Error()+")",
								initTTSAndLaunch,
								skipModelAndContinue(fam),
								quitApp,
							)
							return
						}
					}
					continue
				}

				if !tts.IsModelFamilyInstalled(modelDir, fam, cfg.Audio.UseGPU) {
					spec, _ := tts.ModelSpecByID(fam)
					log.Printf("Model pack %q missing in %s; starting download...", fam, modelDir)
					tracker.SetDownloadModel(spec.ShortName)
					tracker.SetStatus("Downloading " + spec.ShortName + " neural TTS models...")
					if dlErr := tts.DownloadModelFamily(modelDir, fam, cfg.Audio.UseGPU, tracker.SetDownloadProgress); dlErr != nil {
						log.Printf("Error: failed to download %s TTS models: %v", fam, dlErr)
						tracker.ShowFatalError(
							"Failed to download "+spec.ShortName+" TTS models. You can retry, skip to launch, or quit.",
							"Download failed ("+dlErr.Error()+")",
							initTTSAndLaunch,
							skipModelAndContinue(fam),
							quitApp,
						)
						return
					}
				}
			}

			tracker.SetStatus("Initializing shared neural TTS model(s)...")
			threads := cfg.Audio.NumThreads
			if threads <= 0 {
				threads = 4
			}
			eng, engErr := tts.NewMultiEngine(modelDir, enabledModels, threads, cfg.Audio.UseGPU, tts.DefaultMaxClauseWords, 0)
			if engErr != nil {
				log.Printf("Error: failed to initialize TTS engine: %v", engErr)
				tracker.ShowFatalError(
					"Failed to initialize neural TTS engine. You can retry, skip to launch, or quit.",
					"Init error: "+engErr.Error(),
					initTTSAndLaunch,
					func() { launchMainUI(nil) },
					quitApp,
				)
				return
			}

			launchMainUI(eng)
		}

		initTTSAndLaunch()
	}()

	myApp.Run()

	if activeSession != nil {
		_ = activeSession.Close()
		if p := activeSession.Player(); p != nil {
			_ = p.Close()
		}
		if e := activeSession.Engine(); e != nil {
			_ = e.Close()
		}
	} else {
		if activePlayer != nil {
			_ = activePlayer.Close()
		}
		if activeEngine != nil {
			_ = activeEngine.Close()
		}
	}
	return nil
}
