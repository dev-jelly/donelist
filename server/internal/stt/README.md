# Speech-to-Text (STT) Service

This package provides an interface for speech-to-text transcription services that can be integrated into the Donelist mobile app in the future.

## Overview

The STT service is designed to be provider-agnostic, allowing easy integration with various speech recognition services.

## Current Status

- **Status**: Interface defined, not yet integrated into production
- **Current Implementation**: MockProvider (for testing and development)
- **Feature Flag**: Disabled by default

## Supported Providers (Future)

The interface supports integration with any of these providers:

### 1. Google Cloud Speech-to-Text
- **Best for**: High accuracy, multiple languages
- **Pricing**: Pay per 15 seconds of audio
- **Languages**: 125+ languages including Korean
- **Documentation**: https://cloud.google.com/speech-to-text

### 2. AWS Transcribe
- **Best for**: AWS ecosystem integration
- **Pricing**: Pay per second of audio
- **Languages**: 100+ languages including Korean
- **Documentation**: https://aws.amazon.com/transcribe/

### 3. Azure Speech Service
- **Best for**: Microsoft ecosystem integration
- **Pricing**: Pay per hour of audio
- **Languages**: 100+ languages including Korean
- **Documentation**: https://azure.microsoft.com/en-us/services/cognitive-services/speech-to-text/

### 4. OpenAI Whisper
- **Best for**: Open source, offline capability
- **Pricing**: Free (self-hosted) or API pricing
- **Languages**: 99 languages including Korean
- **Documentation**: https://github.com/openai/whisper

## Usage Example

```go
// Create a provider (replace with real implementation)
provider := NewGoogleCloudSTTProvider(apiKey)

// Create STT service
sttService := stt.NewService(provider, true) // enabled=true

// Transcribe audio
result, err := sttService.Transcribe(ctx, audioReader, stt.TranscriptionOptions{
    Language: "ko-KR",
    EnableProfanityFilter: true,
    MaxDuration: 60,
    Format: "webm",
})

if err != nil {
    // Handle error
}

fmt.Printf("Transcribed text: %s (confidence: %.2f)\n", result.Text, result.Confidence)
```

## Integration Steps

To integrate a real STT provider:

1. **Choose a provider** from the list above
2. **Implement the Provider interface**:
   ```go
   type GoogleCloudSTTProvider struct {
       client *speech.Client
   }

   func (g *GoogleCloudSTTProvider) Transcribe(ctx context.Context, audio io.Reader, opts TranscriptionOptions) (*TranscriptionResult, error) {
       // Implement using Google Cloud Speech API
   }
   ```

3. **Add API credentials** to environment configuration:
   ```env
   STT_PROVIDER=google_cloud
   STT_ENABLED=true
   GOOGLE_CLOUD_STT_API_KEY=your_api_key_here
   ```

4. **Update service initialization** in main.go:
   ```go
   var sttProvider stt.Provider
   if config.STT.Provider == "google_cloud" {
       sttProvider = NewGoogleCloudSTTProvider(config.STT.APIKey)
   }

   sttService := stt.NewService(sttProvider, config.STT.Enabled)
   ```

5. **Create API endpoint** for mobile app:
   ```go
   // POST /api/v1/stt/transcribe
   // Content-Type: multipart/form-data
   // Body: audio file + language
   ```

## Validation & Processing

The STT service should integrate with the existing validation package:

```go
// After transcription
result, err := sttService.Transcribe(ctx, audio, opts)
if err != nil {
    return err
}

// Validate transcribed text
if err := validation.ValidateContent(result.Text); err != nil {
    return fmt.Errorf("transcribed content failed validation: %w", err)
}

// Content can now be used to create a check-in
```

## Mobile App Integration

For mobile app (future implementation):

1. **Record audio** on device (iOS/Android)
2. **Compress audio** to reduce upload size (recommended: WebM/Opus)
3. **Upload to API** endpoint with language preference
4. **Receive transcribed text** and pre-fill check-in form
5. **Allow user to edit** before submitting

## Security Considerations

- ✅ Audio files are not stored permanently
- ✅ Transcription results are validated for profanity and spam
- ✅ Maximum audio duration limits prevent abuse
- ✅ Rate limiting should be applied to STT endpoint
- ✅ Support only specific audio formats to prevent malicious files

## Performance Considerations

- Audio transcription typically takes 10-30% of the audio duration
- Consider implementing a queue system for longer audio files
- Cache frequently used phrases/words for faster response
- Implement timeout handling for API calls

## Cost Optimization

- Set maximum audio duration (e.g., 60 seconds)
- Implement client-side audio compression
- Use batch transcription for multiple files
- Cache common phrases/responses
- Consider using open-source Whisper for cost savings

## Testing

The mock provider can be used for testing:

```go
func TestCheckinWithSTT(t *testing.T) {
    mockProvider := stt.NewMockProvider()
    sttService := stt.NewService(mockProvider, true)

    audio := bytes.NewReader(mockAudioData)
    result, err := sttService.Transcribe(context.Background(), audio, stt.TranscriptionOptions{
        Language: "ko-KR",
    })

    require.NoError(t, err)
    assert.NotEmpty(t, result.Text)
}
```

## Future Enhancements

- [ ] Real-time streaming transcription
- [ ] Speaker diarization (multiple speakers)
- [ ] Custom vocabulary for domain-specific terms
- [ ] Automatic language detection
- [ ] Punctuation and formatting
- [ ] Profanity filtering at transcription level
- [ ] Integration with check-in creation flow
