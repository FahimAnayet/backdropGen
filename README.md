# backdropGen

**backdropGen** is an interactive CLI tool written in Go. It automates the creation of video "backdrops" or themes-short, lightweight video loops —designed for media servers like **Jellyfin**, **Emby**, or **Plex**.

It intelligently scans your movie library, picks a high-quality segment from your films, and compresses them into 10-second previews to enhance your library's UI.

---

## Features

* **Batch & Single Mode:** Process an entire root directory with hundreds of movies or target one specific directory.
* **Smart Video Selection:** Automatically identifies the largest video file in a directory (the actual movie) to use as the source.
* **Concurrency:** Built with Go routines to process multiple videos simultaneously, utilizing your CPU efficiently.
* **Interactive UI:** Features a guided terminal interface with directory auto-completion (Tab-to-complete).
* **Audio Control:** 
    * **Always:** Keep the movie's soundtrack.
    * **Never:** Strip audio for silent, atmospheric loops.
    * **Random:** A 30% chance for audio (adds variety).
* **Optimized Encoding:** Downscales 4K/1080p content to 720p and uses a high CRF to keep file sizes tiny (typically under 50MB).
* **Maintenance:** Includes a "Remove All" mode to recursively delete backdrops directorys if you want to refresh your library.

---

## Getting Started

### Prerequisites

* **Go:** 1.18 or higher.
* **FFmpeg & FFprobe:** Must be installed and accessible in your system `$PATH`.

### Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/FahimAnayet/backdropGen.git
   cd backdropGen
   ```

2. **Install dependencies:**
   ```bash
   go get github.com/AlecAivazis/survey/v2
   ```

3. **Build the binary:**
   ```bash
   go build -ldflags="-s -w" -o backdropGen backdropGen.go
   ```

---

## Usage

Run the executable:
```bash
./backdropGen
```

### Options
1.  **Choose an Option:** Select between Batch Creation, Single Creation, or Removal.
2.  **Enter Path:** Provide the path to your media. (e.g., `/mnt/media/Movies` or `~/Videos`).
3.  **Audio Prefs:** Select how you want the tool to handle sound.
4.  **Monitor Progress:** A real-time progress bar tracks the batch processing.

### Debugging
If a specific video is failing to process, run with the debug flag to generate a `debug.log`:
```bash
./backdropGen -d
```

---

## File Structure

The tool follows the standard media server convention for themes:

```text
Films/
└── Interstellar (2014)/
    ├── Interstellar.2160p.mkv
    └── backdrops/            <-- Created by backdropGen
        └── Theme.mp4         <-- 10-second, 720p clip
```

---

## Technical Logic

* **Segment Selection:** The tool picks a random start time at least 2 minutes into the movie (to avoid studio logos) and at least 5 minutes before the end (to avoid credits).
* **FFmpeg Specs:** * **Codec:** `libx264` (Ultrafast preset).
    * **Resolution:** Capped at 720p height (`scale=-2:720`).
    * **Framerate:** Normalized to 24fps.
    * **Bitrate:** Capped at 1M maxrate to ensure fast loading over network streams.

