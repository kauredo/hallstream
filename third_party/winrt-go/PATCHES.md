# winrt-go, patched for 32-bit Windows

A copy of `github.com/saltosystems/winrt-go` at `8ef619c103a3` (MIT, see LICENSE): only the packages tinygo's Bluetooth stack links, without the code generator, the tests or `windows/media`. `go.mod` in the repo root points the module here with a `replace`.

Upstream crashes the 32-bit exe with 0xC0000005 a second after it starts scanning: on 32-bit Windows COM uses stdcall, where the called function pops its own arguments, and upstream gets the argument count wrong in three places.

- `internal/delegate/delegate.go`: every delegate's Invoke was one callback declaring ten parameters. Windows pushes three (the delegate, sender, args), so the callback popped seven words too many on the first scan result. There is now one callback with two arguments and one with none (`DeferralCompletedHandler`, marked by `InvokeTakesNoArgs`).
- `windows/devices/bluetooth/bluetoothledevice.go`: the 64-bit Bluetooth address went in one word, cut in half on 32-bit, which would crash on connect. `internal/abi.U64` splits it.
- `windows/devices/bluetooth/genericattributeprofile/gattserviceprovider.go`, `gattlocalcharacteristic.go`, `gattlocalservice.go`: a GUID passed by value went as a pointer, which only the 64-bit convention does. `internal/abi.GUID` pushes its 16 bytes on 32-bit. Hallstream doesn't call these.

On 64-bit Windows the arguments are laid out exactly as before.

To drop the copy, upstream needs these fixes; then remove the `replace` line and this folder.
