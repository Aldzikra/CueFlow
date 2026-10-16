// Package audio captures the microphone via malgo (miniaudio): mono, 16 kHz,
// signed 16-bit little-endian PCM (Deepgram "linear16").
package audio

import (
	"context"
	"fmt"

	"github.com/gen2brain/malgo"
)

const SampleRate = 16000

// Device is a capture device; Index is what Capture accepts.
type Device struct {
	Index     int
	Name      string
	IsDefault bool
}

func newContext() (*malgo.AllocatedContext, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}
	return ctx, nil
}

func ListDevices() ([]Device, error) {
	ctx, err := newContext()
	if err != nil {
		return nil, err
	}
	defer func() { _ = ctx.Uninit(); ctx.Free() }()
	infos, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return nil, fmt.Errorf("list capture devices: %w", err)
	}
	devs := make([]Device, len(infos))
	for i := range infos {
		devs[i] = Device{i, infos[i].Name(), infos[i].IsDefault != 0}
	}
	return devs, nil
}

// Capture starts recording and returns a channel of PCM chunks. The channel is
// closed when ctx is cancelled. device < 0 selects the system default. If the
// consumer is slow, chunks are dropped rather than blocking the audio thread.
func Capture(ctx context.Context, device int) (<-chan []byte, error) {
	mctx, err := newContext()
	if err != nil {
		return nil, err
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = SampleRate
	if device >= 0 {
		infos, err := mctx.Devices(malgo.Capture)
		if err != nil || device >= len(infos) {
			_ = mctx.Uninit()
			mctx.Free()
			return nil, fmt.Errorf("capture device %d not found (err=%v)", device, err)
		}
		cfg.Capture.DeviceID = infos[device].ID.Pointer()
	}

	out := make(chan []byte, 64)
	dev, err := malgo.InitDevice(mctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			select {
			case out <- append([]byte(nil), in...):
			default:
			}
		},
	})
	if err != nil {
		_ = mctx.Uninit()
		mctx.Free()
		return nil, fmt.Errorf("init capture device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = mctx.Uninit()
		mctx.Free()
		return nil, fmt.Errorf("start capture: %w", err)
	}
	go func() {
		<-ctx.Done()
		dev.Uninit() // stops the callback before we close the channel
		_ = mctx.Uninit()
		mctx.Free()
		close(out)
	}()
	return out, nil
}
