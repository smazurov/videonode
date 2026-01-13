# HDMI Signal Detection Issue - Rockchip RK3588 HDMIRX

## Problem

The V4L2 `GetDVTimings` call returns `no_signal` even though the kernel driver has successfully locked the HDMI signal.

## Evidence

### Kernel (dmesg) shows signal locked:
```
[  792.891861] fdee0000.hdmirx-controller: hdmirx_wait_lock_and_get_timing signal lock ok, i:235!
[  793.279285] rk_hdmirx fdee0000.hdmirx-controller: hdmirx_delayed_work_audio: enable audio
[  793.491193] fdee0000.hdmirx-controller: hdmirx_format_change: New format: 1920x1080p59.08 (2200x1125)
```

### V4L2 query returns no_signal:
```
21:12:13.837[DEBUG][devices]Signal not locked yet, retrying
21:12:14.045[DEBUG][devices]Signal not locked yet, retrying
... (10 retries over ~2 seconds)
21:12:15.706[ERROR][devices]Signal not locked after retries
  device_id=platform-fdee0000.hdmirx-controller-video-index0
  retries=10
  state=no_signal
```

## Timeline

- `792.891s` - Kernel reports signal lock
- `793.491s` - Kernel reports format change (600ms after lock)
- 10 retries at 200ms intervals (~2 seconds total) - V4L2 still reports `no_signal`

## Current Implementation

- `WaitForSourceChange()` - V4L2 event subscription works and receives source change events
- `GetDVTimings()` - Uses `VIDIOC_QUERY_DV_TIMINGS` ioctl to check signal state
- After receiving source change event, we retry `GetDVTimings` 10 times with 200ms delays

## Hypothesis

The `VIDIOC_QUERY_DV_TIMINGS` ioctl may not reflect the actual signal state on the Rockchip HDMIRX driver, or requires additional steps to sync with the hardware state.

## Potential Solutions

1. **Query different V4L2 controls** - Check if there's another ioctl or control that reflects signal state
2. **Increase retry delay** - Try longer delays (500ms+) or more retries
3. **Use `VIDIOC_G_DV_TIMINGS`** instead of `VIDIOC_QUERY_DV_TIMINGS`
4. **Check driver source** - Review rk_hdmirx driver to understand how signal state is exposed

## Files

- `internal/devices/detector_linux.go` - Signal monitoring and event handling
- `pkg/linuxav/v4l2/signal.go` - V4L2 signal detection functions
