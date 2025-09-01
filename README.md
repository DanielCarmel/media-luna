# Media Luna 🎵

**NO AI, NO MAGIC, PURE SCIENCE!**

Media Luna is a Shazam-like audio recognition system implemented in Go that uses advanced audio fingerprinting algorithms to identify songs. The system analyzes audio spectrograms, extracts frequency peaks, and generates unique fingerprints that can be matched against a database of known tracks.

## 🚀 Features

- **Real-time Microphone Recognition**: Just like Shazam - listens from your microphone until it finds a match or times out after 30 seconds
- **File-based Recognition**: Identify songs from audio files (MP3, FLAC, WAV)
- **High-Performance Fingerprinting**: Uses STFT (Short-Time Fourier Transform) and constellation mapping
- **MySQL Database Storage**: Efficient storage and retrieval of audio fingerprints
- **Batch Processing**: Handles large fingerprint datasets with optimized database queries
- **Docker Support**: Easy deployment with Docker Compose

## 🎯 Algorithm Overview

The system implements the core Shazam algorithm:

1. **Audio Processing**: Converts audio to WAV format and generates spectrograms
2. **Peak Detection**: Identifies frequency peaks across 6 frequency bands
3. **Fingerprint Generation**: Creates constellation maps with time-frequency pairs
4. **Hash Generation**: Uses SHA1 to create unique fingerprint hashes
5. **Temporal Matching**: Scores matches based on time alignment consistency

## 📋 Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- MySQL 8.0 (via Docker)
- PortAudio (for microphone input)

On macOS:
```bash
brew install portaudio
```

## 🛠️ Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/DanielCarmel/media-luna.git
   cd media-luna
   ```

2. **Install Go dependencies:**
   ```bash
   go mod tidy
   ```

3. **Start the MySQL database:**
   ```bash
   docker-compose up -d
   ```

4. **Build the application:**
   ```bash
   go build -o eureka cmd/main.go
   ```

## 🎵 Usage

### Adding Songs to Database

Add songs to the database for recognition:

```bash
./eureka -file "path/to/your/song.mp3"
```

### Microphone Recognition (Shazam Mode)

Listen from microphone until a song is recognized or 30-second timeout:

```bash
./eureka -microphone
```

**Example output:**
```
🎤 Recording started... Press Ctrl+C to stop
🎤 Listening for audio... (30s timeout)
🎚️ Audio levels - Max: 0.1077, Avg: 0.0125
🎯 Found 248 peaks from audio
🔑 Generated 6583 fingerprints
✅ Found match: "All Good Things" by Nelly Furtado (Score: 0.892)
```

### File Recognition

Recognize a song from an audio file:

```bash
./eureka -recognize "path/to/unknown/song.mp3"
```

**Example output:**
```
🎵 Found matches:
1. HaGola by Dudu Tasa (Score: 1.000, Offset: 0ms)
```

### Database Management

List all songs in the database:
```bash
./eureka -list
```

Delete a song by ID:
```bash
./eureka -delete 1
```

Clean up duplicate songs:
```bash
./eureka -cleanup
```

## 🏗️ Architecture

```
cmd/main.go                 # CLI interface
internal/
├── eureka/
│   ├── eureka.go          # Core application logic
│   └── recognition.go     # Recognition algorithms
├── fingerprint/
│   ├── fingerprint.go     # Fingerprinting algorithms
│   ├── spectrogram.go     # Spectrogram generation
│   ├── microphone.go      # Real-time audio capture
│   ├── file_format.go     # Audio file processing
│   └── wav_handler.go     # WAV file handling
├── database/
│   └── mysql/
│       └── database_mysql.go  # Database operations
└── common/                # Shared utilities
```

## 📊 Performance

- **Fingerprint Generation**: ~30,000 fingerprints per 30-second audio clip
- **Database Storage**: ~200,000 fingerprints per average 3-minute song
- **Recognition Speed**: < 2 seconds for file recognition
- **Memory Usage**: Efficient batch processing with 1000-hash batches to avoid MySQL limits

## 🎛️ Configuration

The system uses `configs/config.yaml` for database and application settings:

```yaml
database:
  host: localhost
  port: 3306
  user: root
  password: rootpassword
  dbname: eureka
```

## 🐳 Docker Setup

The included `docker-compose.yml` sets up MySQL with persistent storage:

```bash
# Start services
docker-compose up -d

# Stop services
docker-compose down

# View logs
docker-compose logs mysql
```

## 🔬 Technical Details

- **Sample Rate**: 44.1 kHz
- **Window Size**: 4096 samples for STFT
- **Frequency Bands**: 6 bands for peak detection
- **Peak Threshold**: Adaptive (0.02 for microphone, 0.3 for files)
- **Fingerprint Format**: SHA1 hashes of frequency-time pairs
- **Batch Size**: 1000 fingerprints per database query

## 🐛 Troubleshooting

**Microphone not working:**
- Ensure PortAudio is installed
- Check microphone permissions on macOS
- Verify audio input levels

**Database connection errors:**
- Ensure Docker containers are running: `docker-compose ps`
- Check database credentials in config

**MySQL placeholder errors:**
- This has been resolved with batch processing
- Reduce batch size if needed in recognition.go

## 🤝 Contributing

Contributions are welcome! Areas for improvement:

- Enhanced microphone recognition accuracy
- Additional audio format support
- Web interface
- Machine learning-based matching
- Performance optimizations

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Based on the Shazam algorithm research
- Inspired by the original "An Industrial-Strength Audio Search Algorithm" paper
- Built with Go's excellent audio processing libraries
