# linuxav

Pure Go bindings for Linux audio/video device APIs (V4L2, ALSA, udev hotplug).

## Packages

- **v4l2** - Video4Linux2 device enumeration, format/resolution/framerate queries, HDMI signal detection
- **alsa** - ALSA sound card and PCM device enumeration with capability detection
- **hotplug** - Netlink-based device hotplug monitoring (NETLINK_KOBJECT_UEVENT)

## Running Tests

```bash
# Unit tests (fast, no hardware required)
go test -short ./pkg/linuxav/...

# Unit tests with race detector
go test -short -race ./pkg/linuxav/...

# All tests including integration tests
go test ./pkg/linuxav/...
```

## Integration Tests

Integration tests require actual hardware and are skipped in `-short` mode.

### Hotplug Monitor Test

Tests real device hotplug events via netlink socket.

```bash
go test -v -run TestMonitorIntegration -timeout 60s ./pkg/linuxav/hotplug/
```

Then plug/unplug a USB device (webcam, audio interface) within 30 seconds to trigger events.

## Architecture Support

The packages include architecture-specific struct definitions and IOCTL constants for:

- **amd64** (64-bit x86)
- **arm64** (64-bit ARM)
- **arm** (32-bit ARM)

Struct sizes are validated at compile time to ensure ABI compatibility with kernel headers.

## Dependencies

None. Pure Go using only the standard library and syscall interface.
