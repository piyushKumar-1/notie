// Voice notes: record the mic with ffmpeg (avfoundation) and transcribe
// on-device with the `hear` CLI (Apple Speech). Backs the r-prefixed
// commands: radd/rjournal, raddi/rimportant, rremember, rtask.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// recordWav records from the default microphone into a temp wav until the
// user presses Enter, and returns the file path (caller removes it).
func recordWav() string {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		fatal("recording needs ffmpeg — brew install ffmpeg")
	}
	wav := filepath.Join(os.TempDir(), fmt.Sprintf("notie-rec-%d.wav", os.Getpid()))
	cmd := exec.Command(ff, "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-f", "avfoundation", "-i", ":default", "-ac", "1", "-ar", "16000", wav)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fatal("starting ffmpeg: %v", err)
	}
	fmt.Print(cRed + "●" + cReset + " recording — press Enter to stop ")
	bufio.NewReader(os.Stdin).ReadString('\n')
	cmd.Process.Signal(syscall.SIGINT)
	cmd.Wait()
	if st, err := os.Stat(wav); err != nil || st.Size() == 0 {
		os.Remove(wav)
		fatal("no audio captured — check the microphone permission for your terminal")
	}
	return wav
}

// whisperModel returns the whisper model path: $NOTIE_WHISPER_MODEL, else
// the best model present in ~/.cache/whisper.
func whisperModel() string {
	if m := os.Getenv("NOTIE_WHISPER_MODEL"); m != "" {
		return m
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cache", "whisper")
	names := []string{"ggml-large-v3-turbo.bin", "ggml-medium.bin", "ggml-small.bin", "ggml-base.bin"}
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return filepath.Join(dir, n)
		}
	}
	return filepath.Join(dir, names[len(names)-1])
}

// transcribe turns a wav file into one line of text, via the hear CLI
// (Apple Speech) when installed, else whisper-cli with a local model.
func transcribe(wav string) string {
	if bin, err := exec.LookPath("hear"); err == nil {
		if out, err := exec.Command(bin, "-i", wav).Output(); err == nil {
			return strings.Join(strings.Fields(string(out)), " ")
		}
	}
	bin, err := exec.LookPath("whisper-cli")
	if err != nil {
		fatal("transcription needs whisper-cli — brew install whisper-cpp")
	}
	model := whisperModel()
	if _, err := os.Stat(model); err != nil {
		fatal("whisper model missing at %s — download one from huggingface.co/ggerganov/whisper.cpp", model)
	}
	out, err := exec.Command(bin, "-m", model, "-f", wav, "-np", "-nt").Output()
	if err != nil {
		fatal("whisper-cli: %v", err)
	}
	return strings.Join(strings.Fields(string(out)), " ")
}

// cmdRecord records + transcribes, lets the user confirm or correct the
// transcript, then routes it to the target store.
func cmdRecord(target string) {
	wav := recordWav()
	defer os.Remove(wav)
	fmt.Println(cGrey + "transcribing…" + cReset)
	text := transcribe(wav)
	// whisper marks non-speech as e.g. "[BLANK_AUDIO]" or "(wind blowing)"
	if text == "" ||
		(strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")) ||
		(strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")")) {
		fatal("heard nothing")
	}
	fmt.Println("  " + cBold + text + cReset)
	fmt.Print(cGrey + "[Enter] save · [n] discard · or type a correction: " + cReset)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		fmt.Println("discarded")
		return
	}
	switch line = strings.TrimSpace(line); line {
	case "":
	case "n":
		fmt.Println("discarded")
		return
	default:
		text = line
	}
	switch target {
	case "journal":
		cmdAdd(today(), clock(), text)
	case "important":
		cmdDated("important.md", "Important", "important", text)
	case "remember":
		cmdDated("remember.md", "Remember", "remember", text)
	case "task":
		// priority is mandatory for tasks; keep asking until we get one
		in := bufio.NewReader(os.Stdin)
		for {
			fmt.Print(cGrey + "priority [0 high · 1 normal · 2 low]: " + cReset)
			p, err := in.ReadString('\n')
			if err != nil {
				fmt.Println("discarded")
				return
			}
			if p = strings.TrimSpace(p); p == "0" || p == "1" || p == "2" {
				cmdTask([]string{p, text})
				return
			}
		}
	}
}
