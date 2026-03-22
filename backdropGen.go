package main

import (
	"fmt"
	"io/fs"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlecAivazis/survey/v2"
)

// Global Debug Flag
var debugMode bool
var logger *log.Logger

type AudioConfig int

const (
	AudioAlways AudioConfig = iota
	AudioNever
	AudioRandom
)

type VideoJob struct {
	dir   string
	index int
}

type ProgressTracker struct {
	mu      sync.Mutex
	current int
	total   int
	label   string
}

func (p *ProgressTracker) increment() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	p.printProgress()
}

func (p *ProgressTracker) printProgress() {
	percent := float64(p.current) / float64(p.total) * 100
	barLength := 40
	filled := int(float64(barLength) * percent / 100)
	bar := strings.Repeat("#", filled) + strings.Repeat("-", barLength-filled)
	fmt.Printf("\r%s |%s| %d%% (%d/%d)", p.label, bar, int(percent), p.current, p.total)
	if p.current == p.total {
		fmt.Println()
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())

	// Check for debug flags
	for _, arg := range os.Args {
		if arg == "--debug" || arg == "-d" {
			debugMode = true
		}
	}

	if debugMode {
		file, err := os.OpenFile("debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Println("Failed to create log file, debug mode disabled.")
			debugMode = false
		} else {
			logger = log.New(file, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
			fmt.Println("!!! Debug mode enabled. Logging to debug.log !!!")
		}
	}
}

func main() {
	var choice string
	menuPrompt := &survey.Select{
		Message: "Choose an option:",
		Options: []string{
			"1. Create Backdrops (Batch - Root folder with subfolders)",
			"2. Create Backdrop (Single - Specific Movie folder)",
			"3. Remove All Backdrops",
			"Quit",
		},
	}

	err := survey.AskOne(menuPrompt, &choice)
	if err != nil || choice == "Quit" {
		fmt.Println("Exiting...")
		os.Exit(0)
	}

	root := askForPath()
	if root == "" {
		os.Exit(0)
	}

	if strings.Contains(choice, "Remove") {
		removeBackdrops(root)
		return
	}

	// Audio Selection
	var audioChoice string
	audioPrompt := &survey.Select{
		Message: "Audio Options:",
		Options: []string{"Always Include Audio", "Never Include Audio", "Random (30% chance)", "Quit"},
	}

	err = survey.AskOne(audioPrompt, &audioChoice)
	if err != nil || audioChoice == "Quit" {
		os.Exit(0)
	}

	var audioPref AudioConfig
	switch {
	case strings.Contains(audioChoice, "Always"):
		audioPref = AudioAlways
	case strings.Contains(audioChoice, "Never"):
		audioPref = AudioNever
	default:
		audioPref = AudioRandom
	}

	if strings.Contains(choice, "Single") {
		// Single Mode
		fmt.Println("Processing single directory...")
		createBackdrop(root, audioPref)
		fmt.Println("Done.")
	} else {
		// Batch Mode
		createBackdrops(root, audioPref)
	}
}

// --- CORE LOGIC ---

func createBackdrops(root string, audioPref AudioConfig) {
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	}

	// Collect dirs that directly contain at least one video file
	dirSet := make(map[string]bool)
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if videoExts[strings.ToLower(filepath.Ext(d.Name()))] {
			dirSet[filepath.Dir(path)] = true
		}
		return nil
	})

	var validDirs []string
	for dir := range dirSet {
		validDirs = append(validDirs, dir)
	}

	if len(validDirs) == 0 {
		fmt.Println("No directories to process.")
		return
	}

	numWorkers := 4
	if runtime.NumCPU() < numWorkers {
		numWorkers = runtime.NumCPU()
	}

	jobs := make(chan VideoJob, len(validDirs))
	var wg sync.WaitGroup
	tracker := &ProgressTracker{total: len(validDirs), label: "Processing"}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				createBackdrop(job.dir, audioPref)
				tracker.increment()
			}
		}()
	}

	for i, dir := range validDirs {
		jobs <- VideoJob{dir: dir, index: i}
	}
	close(jobs)
	wg.Wait()
}

func createBackdrop(dir string, audioPref AudioConfig) {
    video := findLargestVideoFile(dir)

    if debugMode {
        logger.Printf("Checking Directory: %s", dir)
        logger.Printf("Found Video: %s", video)
    }

    if video == "" {
        if debugMode {
            logger.Printf("SKIP: No video file found in %s", dir)
        }
        return
    }

	backdropDir := filepath.Join(filepath.Dir(video), "backdrops")
	
	if _, err := os.Stat(backdropDir); err == nil {
        if debugMode {
            logger.Printf("SKIP: Backdrop already exists at %s", backdropDir)
        }
        return
    }

	width, height, duration := getVideoInfo(video)

	if debugMode {
		logger.Printf("Video Info: W:%d H:%d Dur:%.2f", width, height, duration)
	}

	// If you want to test on short videos, lower the 420 (7 mins) to 10
	if width == 0 || height == 0 || duration <= 420 {
		if debugMode {
			logger.Printf("SKIP: Video too short or invalid metadata.")
		}
		return
	}

	start := randomStartTime(duration)
	output := filepath.Join(backdropDir, "Theme.mp4")
	os.MkdirAll(backdropDir, os.ModePerm)

	args := []string{
		"-y", "-loglevel", "error", "-hide_banner",
		"-ss", fmt.Sprintf("%.2f", start),
		"-i", video,
		"-t", "10",
		"-map", "0:v:0",
	}

	includeAudio := false
	switch audioPref {
	case AudioAlways:
		includeAudio = true
	case AudioNever:
		includeAudio = false
	case AudioRandom:
		includeAudio = rand.Float64() < 0.3
	}

	if includeAudio {
		args = append(args, "-map", "0:a:0?", "-c:a", "aac", "-b:a", "96k")
	}

	args = append(args,
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "30",
		"-maxrate", "1M", "-bufsize", "2M", "-r", "24",
	)

	if height > 720 {
		args = append(args, "-vf", "scale=-2:720")
	}
	args = append(args, "-f", "mp4", output)

	cmd := exec.Command("ffmpeg", args...)
	
	if debugMode {
		logger.Printf("Running FFmpeg: ffmpeg %s", strings.Join(args, " "))
		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Printf("FFMPEG ERROR: %v\nOutput: %s", err, string(output))
		} else {
			logger.Printf("SUCCESS: Backdrop created at %s", output)
		}
	} else {
		cmd.Run()
	}
}

// --- PATH HELPERS ---

func expandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, path[1:]), nil
}

func askForPath() string {
	target := ""
	prompt := &survey.Input{
		Message: "Enter root directory (Tab for suggestions, 'q' to quit):",
		Suggest: func(toComplete string) []string {
			expanded, _ := expandPath(toComplete)
			files, _ := filepath.Glob(expanded + "*")
			return files
		},
	}

	err := survey.AskOne(prompt, &target)
	if err != nil {
		if err.Error() == "interrupt" {
			fmt.Println("\nGoodbye!")
			os.Exit(0)
		}
		return ""
	}

	cleanTarget := strings.TrimSpace(target)
	if strings.ToLower(cleanTarget) == "q" {
		return ""
	}

	root, err := expandPath(cleanTarget)
	if err != nil {
		fmt.Println("Error: Could not resolve home directory.")
		return ""
	}

	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Error: Directory does not exist.")
		} else {
			fmt.Printf("Error accessing directory: %v\n", err)
		}
		return ""
	}

	if !info.IsDir() {
		fmt.Println("Error: Path is a file, not a directory.")
		return ""
	}

	return root
}


func removeBackdrops(root string) {
	var toRemove []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d != nil && d.IsDir() && d.Name() == "backdrops" {
			toRemove = append(toRemove, path)
		}
		return nil
	})

	var wg sync.WaitGroup
	for _, path := range toRemove {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if err := os.RemoveAll(p); err == nil {
				fmt.Println("Removed:", p)
			}
		}(path)
	}
	wg.Wait()

	if len(toRemove) == 0 {
		fmt.Println("No Backdrops directories found.")
	}
}

func findLargestVideoFile(dir string) string {
	var largestFile string
	var maxSize int64
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
		".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	}

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !videoExts[ext] {
			return nil
		}
		info, _ := d.Info()
		if info.Size() > maxSize {
			maxSize = info.Size()
			largestFile = path
		}
		return nil
	})
	return largestFile
}

func getVideoInfo(file string) (int, int, float64) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration:format=duration",
		"-of", "csv=p=0:s=,",
		file)

	output, err := cmd.Output()
	if err != nil {
		if debugMode {
			logger.Printf("FFPROBE EXCEPTION for %s: %v", file, err)
		}
		return 0, 0, 0
	}

	// Output is two lines: "width,height,streamDur" then "formatDur"
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 1 {
		return 0, 0, 0
	}

	streamParts := strings.Split(strings.TrimSpace(lines[0]), ",")
	if len(streamParts) < 2 {
		return 0, 0, 0
	}

	width, _ := strconv.Atoi(streamParts[0])
	height, _ := strconv.Atoi(streamParts[1])

	// Try stream duration first (index 2 of stream line)
	var duration float64
	if len(streamParts) > 2 {
		duration, _ = strconv.ParseFloat(strings.TrimSpace(streamParts[2]), 64)
	}

	// Fallback to format duration (second line)
	if duration == 0 && len(lines) > 1 {
		duration, _ = strconv.ParseFloat(strings.TrimSpace(lines[1]), 64)
	}

	return width, height, duration
}


func randomStartTime(duration float64) float64 {
	start := 120.0
	end := duration - 310.0
	if end <= start {
		return start
	}
	return start + rand.Float64()*(end-start)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
