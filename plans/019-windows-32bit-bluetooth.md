# 019: The 32-bit Windows exe stops crashing on Bluetooth

Status: built and tried on the club laptop; in review.

Repo `app/`, on top of plan 018. While trying plan 018 on the club laptop on 2026-11-27, `Hallstream (32-bit).exe` died with 0xC0000005 about a second after "À procura da Stream Box...", with nothing on stderr and no crash record. The released v0.1.7 32-bit exe did the same, and both ran with `-demo`. The owner asked to fix it in the same release.

## Facts found

- The Bluetooth stack is `tinygo.org/x/bluetooth` v0.17.0, which calls WinRT through `github.com/saltosystems/winrt-go` (latest upstream, `8ef619c103a3`, has the same code).
- `winrt-go/internal/delegate` registers one Invoke callback for every delegate, declared with ten parameters. On 32-bit Windows COM uses stdcall, where the called function pops its own arguments. WinRT calls Invoke with three (the delegate, sender, args), so the callback pops seven words too many and the caller's stack breaks on the first scan result. On 64-bit the caller cleans up and the extra parameters are harmless, which is why only the 32-bit exe crashes.
- The three delegate types are `TypedEventHandler` (sender, args), `AsyncOperationCompletedHandler` (operation, status) and `DeferralCompletedHandler` (none).
- `BluetoothLEDeviceFromBluetoothAddressAsync` (and the variant with an address type) pass the 64-bit address as one word. On 32-bit that halves it and shifts the arguments after it, so connecting to the box would crash next. Three GATT server calls pass a GUID as a pointer, which only the 64-bit convention does; Hallstream doesn't call them.

## Decisions

| # | Rating | Question | Pick | Why / risk |
|---|---|---|---|---|
| 1 | ✅ | How to ship the fix | A copy of the runtime packages of winrt-go in `third_party/winrt-go` (560 KB, no generator), patched, with a `replace` in `go.mod`. `PATCHES.md` lists every change. | Upstream has no fix; a fork on GitHub would be a second repo to keep. |
| 2 | ✅ | Delegates | One Invoke callback with two arguments and one with none, chosen per delegate (`DeferralCompletedHandler` is marked). | Matches what WinRT pushes on every architecture. |
| 3 | ✅ | By-value arguments | `internal/abi.U64` and `abi.GUID` lay them out per architecture; 64-bit is unchanged. | The two address calls and the three GUID calls. |
| 4 | 🔥 | Send the fix upstream | Not from here: it would be a PR on someone else's repo. The owner decides. | Once upstream has it, drop the copy. |
| 5 | ✅ | The 32-bit WebView2 window | Stays off (plan 018): its hang was inside WebView2's start, not in this code. | Unproven either way; Edge works. |

## Done when

The 32-bit exe keeps scanning for the box on Windows instead of crashing, and the 64-bit exe behaves as before.

## Tried before merge

On the club laptop, the patched 32-bit exe scanned for over a minute and logged 59 devices in `bluetooth.log` with no crash. Connecting to a Stream Box couldn't be tried there (no box in range); the address fix is checked by the build only.
