package decoder

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// WavHeader represents the header of a WAV file
type WavHeader struct {
	ChunkID       [4]byte
	ChunkSize     uint32
	Format        [4]byte
	Subchunk1ID   [4]byte
	Subchunk1Size uint32
	AudioFormat   uint16
	NumChannels   uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}

// Decode reads a WAV file and attempts to decode Apple ][ data
func Decode(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var header WavHeader
	if err := binary.Read(f, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("failed to read WAV header: %w", err)
	}

	fmt.Printf("WAV Header: %+v\n", header)

	if string(header.ChunkID[:]) != "RIFF" || string(header.Format[:]) != "WAVE" {
		return nil, fmt.Errorf("invalid WAV file")
	}

	// Find the data chunk
	for {
		var chunkID [4]byte
		var chunkSize uint32
		if err := binary.Read(f, binary.LittleEndian, &chunkID); err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("data chunk not found")
			}
			return nil, err
		}
		if err := binary.Read(f, binary.LittleEndian, &chunkSize); err != nil {
			return nil, err
		}

		if string(chunkID[:]) == "data" {
			break // Found data chunk
		}

		// Skip other chunks
		if _, err := f.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
			return nil, err
		}
	}

	// Read samples
	// Assuming 8-bit unsigned or 16-bit signed PCM
	// We'll convert everything to float64 for easier processing
	var samples []float64

	if header.BitsPerSample == 8 {
		// 8-bit samples are unsigned 0-255, center at 128
		buf := make([]byte, 1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				for i := 0; i < n; i += int(header.NumChannels) {
					// Use Left channel (first sample)
					sample := (float64(buf[i]) - 128.0) / 128.0
					samples = append(samples, sample)
				}
			}
			if err != nil {
				break
			}
		}
	} else if header.BitsPerSample == 16 {
		// 16-bit samples are signed -32768 to 32767
		// Read all channels
		buf := make([]int16, 1024)
		for {
			err := binary.Read(f, binary.LittleEndian, &buf)
			if err == nil {
				for i := 0; i < len(buf); i += int(header.NumChannels) {
					// Use Left channel (first sample)
					if i < len(buf) {
						sample := float64(buf[i]) / 32768.0
						samples = append(samples, sample)
					}
				}
			} else {
				break
			}
		}
	} else {
		return nil, fmt.Errorf("unsupported bits per sample: %d", header.BitsPerSample)
	}

	fmt.Printf("Read %d samples\n", len(samples))

	// Zero-crossing analysis
	return processSamples(samples, header.SampleRate), nil
}

func processSamples(samples []float64, sampleRate uint32) []byte {
	var crossings []int
	prevSample := samples[0]

	for i, sample := range samples {
		if (prevSample < 0 && sample >= 0) || (prevSample >= 0 && sample < 0) {
			crossings = append(crossings, i)
		}
		prevSample = sample
	}

	fmt.Printf("Detected %d zero crossings\n", len(crossings))

	// State machine
	const (
		StateFindHeader = iota
		StateFindSync
		StateReadData
	)

	state := StateFindHeader
	var decodedBytes []byte
	var currentByte byte
	var bitCount int

	// Thresholds
	const (
		ShortThreshold = 0.000350 // 350us
		LongThreshold  = 0.000600 // 600us
	)

	// We iterate through half-cycles.
	// We need pairs of half-cycles to form a bit.
	// Ideally, they should match (Short+Short or Long+Long).

	headerCount := 0

	for i := 1; i < len(crossings); i++ {
		durationSamples := crossings[i] - crossings[i-1]
		durationSec := float64(durationSamples) / float64(sampleRate)

		var isShort, isHeader bool
		if durationSec < ShortThreshold {
			isShort = true
		} else if durationSec < LongThreshold {
			// isLong = true
		} else {
			isHeader = true
		}

		switch state {
		case StateFindHeader:
			if isHeader {
				headerCount++
			} else {
				// If we had enough header tone, and now we see a Short, it might be the sync bit
				if headerCount > 100 && isShort {
					// Potential sync bit start
					// We need another Short to confirm sync bit (0 is Short+Short)
					state = StateFindSync
				} else {
					headerCount = 0
				}
			}
		case StateFindSync:
			if isShort {
				// Second half of sync bit found!
				// fmt.Println("Sync bit found! Starting data decode...")
				state = StateReadData
				currentByte = 0
				bitCount = 0
			} else {
				// False alarm, go back to finding header
				state = StateFindHeader
				headerCount = 0
			}
		case StateReadData:
			// We need to read pairs.
			// This is a simplified approach: we just look at the current half-cycle.
			// A more robust approach would buffer the next half-cycle and check consistency.
			// But for now, let's assume if we see a Short, we expect another Short.
			// If we see a Long, we expect another Long.

			// Actually, let's just peek at the next one if possible, or maintain state.
			// Let's use a sub-state or just skip the next one if it matches.

			// Better: Read two half-cycles at a time?
			// The loop is iterating one by one.
			// Let's just track "first half" vs "second half".

			// Wait, the loop index `i` is for the current half-cycle.
			// Let's skip the loop index manipulation and just use a flag.
		}
	}

	// Re-implementing the loop to handle pairs properly
	i := 1
	state = StateFindHeader
	headerCount = 0

	for i < len(crossings) {
		durationSamples := crossings[i] - crossings[i-1]
		durationSec := float64(durationSamples) / float64(sampleRate)
		i++ // Move to next

		var isShort, isHeader bool
		if durationSec < ShortThreshold {
			isShort = true
		} else if durationSec < LongThreshold {
			// isLong = true
		} else {
			isHeader = true
		}

		switch state {
		case StateFindHeader:
			// Accept Header (> 600us) or Long (1000Hz, ~500us) as header tone
			if isHeader || (durationSec > ShortThreshold && durationSec < LongThreshold) {
				headerCount++
			} else {
				// If we had enough header tone, and now we see a Short, it might be the sync bit
				if headerCount > 50 && isShort { // Reduced header requirement for testing
					// Check next half-cycle for Sync (Short+Short)
					if i < len(crossings) {
						nextDur := float64(crossings[i]-crossings[i-1]) / float64(sampleRate)
						if nextDur < ShortThreshold {
							// Sync confirmed
							// fmt.Println("Sync bit found!")
							state = StateReadData
							currentByte = 0
							bitCount = 0
							i++ // Consumed the second half of sync
						} else {
							state = StateFindHeader
							headerCount = 0
						}
					}
				} else {
					headerCount = 0
				}
			}
		case StateReadData:
			// Read a bit (2 half cycles)
			if i >= len(crossings) {
				break
			}

			// We already have the first half in `durationSec` (from before i++),
			// but wait, I incremented i already.
			// Let's step back. `durationSec` is `crossings[i-1] - crossings[i-2]`.
			// We need the second half.

			dur1 := durationSec
			dur2Samples := crossings[i] - crossings[i-1]
			dur2 := float64(dur2Samples) / float64(sampleRate)
			i++ // Consume second half

			// Determine bit
			// 0 = Short + Short
			// 1 = Long + Long

			isZero := dur1 < ShortThreshold && dur2 < ShortThreshold
			isOne := (dur1 >= ShortThreshold && dur1 < LongThreshold) && (dur2 >= ShortThreshold && dur2 < LongThreshold)

			if isZero {
				// 0 bit
				// Apple II data is MSB first? No, usually LSB first in some formats, but Monitor is MSB?
				// Actually, standard Monitor `RDBYTE` shifts bits in.
				// It does `ROL` (Rotate Left), so new bit goes into LSB, and everything shifts left?
				// Wait, `ROL` shifts Carry into LSB, and MSB into Carry.
				// The routine reads 8 bits.
				// Let's assume MSB first for now (shifting into LSB means the first bit read ends up at MSB? No.)
				// If I read B1, shift left -> B1.
				// Read B2, shift left -> B1 B2.
				// ...
				// Read B8, shift left -> B1 B2 ... B8.
				// So B1 is MSB.

				// "0" bit
				currentByte = (currentByte << 1) // | 0
				bitCount++
			} else if isOne {
				// "1" bit
				currentByte = (currentByte << 1) | 1
				bitCount++
			} else {
				// Error or end of data
				// fmt.Printf("Bit error at %d: %.6f, %.6f\n", i, dur1, dur2)
				// For now, let's just ignore or reset?
				// If it's a Header tone, maybe we finished?
				if dur1 > LongThreshold || dur2 > LongThreshold {
					// fmt.Println("End of data (header tone found)")
					state = StateFindHeader
					headerCount = 0
				}
			}

			if bitCount == 8 {
				decodedBytes = append(decodedBytes, currentByte)
				currentByte = 0
				bitCount = 0
			}
		}
	}

	return decodedBytes
}
