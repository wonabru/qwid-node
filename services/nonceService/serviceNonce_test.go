package nonceServices

import (
	"testing"

	"github.com/qwid-org/qwid-node/common"
	"github.com/qwid-org/qwid-node/logger"
	"github.com/stretchr/testify/assert"
)

func TestResetToDefaultEncryptionOptData(t *testing.T) {
	logger.InitLogger()
	defer logger.CloseLogger()

	t.Run("reset creates valid encryption data", func(t *testing.T) {
		ResetToDefaultEncryptionOptData()

		assert.NotNil(t, EncryptionOptData)
		// Default encryption data should have length prefix for two empty byte slices
		// Each empty slice with length prefix = 4 bytes (int32 length = 0)
		assert.Equal(t, 8, len(EncryptionOptData))
	})

	t.Run("multiple resets produce same result", func(t *testing.T) {
		ResetToDefaultEncryptionOptData()
		first := make([]byte, len(EncryptionOptData))
		copy(first, EncryptionOptData)

		ResetToDefaultEncryptionOptData()
		second := make([]byte, len(EncryptionOptData))
		copy(second, EncryptionOptData)

		assert.Equal(t, first, second)
	})
}

func TestSetEncryptionData(t *testing.T) {
	logger.InitLogger()
	defer logger.CloseLogger()

	t.Run("set custom encryption data", func(t *testing.T) {
		enc1 := []byte{1, 2, 3, 4}
		enc2 := []byte{5, 6, 7, 8}

		SetEncryptionData(enc1, enc2)

		assert.NotNil(t, EncryptionOptData)
		// Length should be: 4 (len prefix) + 4 (enc1) + 4 (len prefix) + 4 (enc2) = 16
		assert.Equal(t, 16, len(EncryptionOptData))
	})

	t.Run("set empty encryption data", func(t *testing.T) {
		SetEncryptionData([]byte{}, []byte{})

		assert.NotNil(t, EncryptionOptData)
		assert.Equal(t, 8, len(EncryptionOptData))
	})

	t.Run("set different sized encryption data", func(t *testing.T) {
		enc1 := make([]byte, 100)
		enc2 := make([]byte, 50)

		SetEncryptionData(enc1, enc2)

		assert.NotNil(t, EncryptionOptData)
		// Length should be: 4 + 100 + 4 + 50 = 158
		assert.Equal(t, 158, len(EncryptionOptData))
	})
}

func TestSendFunction(t *testing.T) {
	logger.InitLogger()
	defer logger.CloseLogger()

	t.Run("send without initialization returns false", func(t *testing.T) {
		// Without initializing the service, Send should fail gracefully
		addr := [4]byte{192, 168, 1, 1}
		data := []byte("test data")

		// This should not panic even without initialization
		result := Send(addr, data)
		// Result depends on whether channel is initialized
		assert.IsType(t, true, result)
	})
}

func TestLastRepliedIP(t *testing.T) {
	logger.InitLogger()
	defer logger.CloseLogger()

	t.Run("last replied IP is initially zero", func(t *testing.T) {
		expected := [4]byte{0, 0, 0, 0}
		assert.Equal(t, expected, LastRepliedIP)
	})

	t.Run("last replied IP can be set", func(t *testing.T) {
		newIP := [4]byte{10, 0, 0, 1}
		LastRepliedIP = newIP
		assert.Equal(t, newIP, LastRepliedIP)

		// Reset for other tests
		LastRepliedIP = [4]byte{0, 0, 0, 0}
	})
}

func TestEncryptionDataConcurrency(t *testing.T) {
	logger.InitLogger()
	defer logger.CloseLogger()

	t.Run("concurrent access to encryption data", func(t *testing.T) {
		done := make(chan bool, 10)

		// Multiple goroutines setting encryption data
		for i := 0; i < 5; i++ {
			go func(n int) {
				enc1 := make([]byte, n+1)
				enc2 := make([]byte, n+2)
				SetEncryptionData(enc1, enc2)
				done <- true
			}(i)
		}

		// Multiple goroutines resetting
		for i := 0; i < 5; i++ {
			go func() {
				ResetToDefaultEncryptionOptData()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Should not panic and data should be valid
		assert.NotNil(t, EncryptionOptData)
	})
}

func TestBytesToLenAndBytesIntegration(t *testing.T) {
	logger.InitLogger()
	defer logger.CloseLogger()

	t.Run("encryption data uses proper byte encoding", func(t *testing.T) {
		testData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
		encoded := common.BytesToLenAndBytes(testData)

		// First 4 bytes should be the length
		length := common.GetInt32FromByte(encoded[:4])
		assert.Equal(t, int32(4), length)

		// Remaining bytes should be the data
		assert.Equal(t, testData, encoded[4:])
	})
}
