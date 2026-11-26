# Two or three cameras

This guide is for a club that wants to film the game with more than one camera. With a single camera, READ-ME.txt is enough.

## Before you start

Hallstream puts each camera in its own OBS scene, with the graphics on top. **Screens** shows one button per camera, and the one on air says "On air". Click another button to change camera.

You can have up to three cameras. Each one can be:

- **A phone with DroidCam**: an app on the phone that sends the picture to OBS, over Wi-Fi or a USB cable.
- **A camera plugged into the PC**: a USB webcam, or a camera with an HDMI output plugged into a capture card.
- **A phone with no app (VDO.Ninja)**: the phone opens a page in its browser and sends the picture through it.

Install OBS 31 or newer and, for DroidCam, the app on the phone and the DroidCam OBS plugin on the PC (https://droidcam.app/obs/).

## Choosing the cameras

    Camera                 App   Internet  Cable   Cost
    DroidCam               yes   no        USB     HD only with Pro
    Camera plugged in      no    no        always  the camera's
    Phone (VDO.Ninja)      no    yes       no      free

- **DroidCam**: the free version stays at 480p. For 720p or 1080p with no watermark you need to buy DroidCam Pro, in the phone app. For several phones, each one is a DroidCam source in OBS.
- **Camera plugged into the PC**: it doesn't use the network. It is the steadiest.
- **Phone (VDO.Ninja)**: nothing to install, but it needs the internet on the PC and on the phone during the game. The rest of Hallstream works with no internet.

## How much the network carries

Every phone on Wi-Fi takes a share of the hall's Wi-Fi. These are the numbers from DroidCam's and VDO.Ninja's pages, per camera:

    Per camera                            Mbps
    DroidCam 480p (free version)          0.6 to 1.4
    DroidCam 720p, Normal quality         4
    DroidCam 1080p, Normal quality        7
    Phone (VDO.Ninja)                     4, plus a margin

Hallstream asks VDO.Ninja for 4000 kbps (4 Mbps) per phone, with no sound. VDO.Ninja says audio and overhead come on top, with no number. Allow a margin.

Adding the cameras up:

    Cameras on Wi-Fi                      Two     Three
    DroidCam 720p, Normal                 8       12
    DroidCam 1080p, Normal                14      21
    Phone (VDO.Ninja)                     8+      12+

On top of this, the PC sends the stream to YouTube. For 1080p at 30 frames per second, YouTube recommends 14 Mbps (H.264), with a minimum of 5 Mbps. That upload uses the hall's internet, and the Wi-Fi too if the PC is on Wi-Fi. Plug the PC into the router with a network cable whenever you can.

One rule: if the total goes past what the hall's Wi-Fi gives, or the picture breaks up in a rehearsal, put a phone on a cable or drop one camera to 720p. DroidCam recommends 720p as the best balance of quality and performance.

If you don't know what the Wi-Fi gives, rehearse before the first game, with every camera running at once for a few minutes.

## Two cameras on Wi-Fi

1. Plug the PC into the router with a network cable, if you can.
2. Connect both phones to the same Wi-Fi network as the PC. If the router has a 2.4 GHz network and a 5 GHz one, put them all on the same one.
3. Put each phone on its charger, somewhere its heat can get away (not in the sun, not against something warm). DroidCam asks for the battery at 100% and on power.
4. Open DroidCam on both phones. Each shows its "WiFi IP" address.
5. In the panel, open **Settings > Cameras**. For **Camera 1**, under **Where the picture comes from**, pick the DroidCam source. Under **Name on the panel**, type a short name, for example `Stand`.
6. Click **Add camera** and pick the source for **Camera 2** the same way. If the second phone isn't in OBS yet, pick DroidCam under **Add to OBS**.
7. Click **Save and set up OBS**. Wait for the list in the OBS card to say it is ready.
8. In OBS, open the properties of each DroidCam source that has no picture yet: tick **Use WiFi**, type that phone's "WiFi IP" and click **Activate**.
9. On **Screens**, check that both buttons say "Picture OK".

## Three cameras on Wi-Fi

Follow the steps above for all three cameras. Then:

1. Look at the totals in "How much the network carries". Three phones at 1080p are 21 Mbps of picture alone.
2. If the picture breaks up, drop to 720p first the camera that is on air least (the bench one, for example). In DroidCam you change it in the app's settings or in the source's properties in OBS.
3. If it still breaks up, put the main camera on a cable (next section).

## On cable

A cable takes that camera off the Wi-Fi: the picture goes through the cable. DroidCam says USB is more stable, because there is no interference and no packet loss.

### Android phone with DroidCam, over USB

1. On the phone, open **Settings** and search for **Build number**, usually under **About phone**.
2. Tap **Build number** seven times. This turns on **Developer options**.
3. Go back to **Settings**, search for **USB debugging** and turn it on.
4. Plug the phone into the PC with the USB cable. On the phone, accept **Allow USB debugging**.
5. Open DroidCam on the phone and, in the panel, click **Save and set up OBS**. Hallstream connects the DroidCam source to the first USB phone that has no source yet.

### iPhone with DroidCam, over USB

1. Use an Apple cable or an Apple-approved one.
2. On the PC, check that **Apple Mobile Device Support** is installed: press the Windows key, search for **Installed apps** and look for it in the list. Windows usually installs it by itself when you plug the iPhone in.
3. Plug the iPhone into the PC. If the iPhone asks whether to trust this computer, tap **Trust**.
4. Open DroidCam on the iPhone and, in the panel, click **Save and set up OBS**.

### Webcam or capture card

Plug it into a USB port on the PC. In **Settings > Cameras**, pick it under **Where the picture comes from** (under **Add to OBS**, if it isn't in OBS yet) and click **Save and set up OBS**.

### The PC on a network cable

Plug the PC into the router with a network cable. The phones on Wi-Fi still reach the PC, as long as they are on the same router.

### A mix

A good layout: the main camera on a cable (DroidCam over USB or a camera plugged into the PC) and the others on Wi-Fi.

## A phone with no app (VDO.Ninja)

It needs the internet on the PC and on the phone. VDO.Ninja says the picture usually stays on the hall's network, but loading the page and connecting the phone to the PC need the internet.

1. In **Settings > Cameras**, for a camera, pick **Phone (VDO.Ninja)** under **Where the picture comes from**.
2. Click **Save and set up OBS**. A QR code appears under that camera.
3. On the phone that will film, scan the QR code with the phone's camera and open the link in the browser.
4. Let the page use the camera when the browser asks.
5. Turn the phone sideways, put it on the charger and leave the screen on, with the page open.
6. On **Screens**, check that this camera says "Picture OK".

One QR code serves one phone. VDO.Ninja refuses a second phone on the same code. For another phone, add another camera.

The code doesn't change: the same phone can scan the same code at the next game. Keep it like a secret, because anyone with the code can send a picture to that camera.

## On game day

1. Put the phones on their chargers and open DroidCam, or the VDO.Ninja page, before you open Hallstream.
2. On **Screens**, look at each camera's line:
   - **Picture OK**: all is well.
   - **No picture**: OBS gets nothing from that camera, or gets a picture all one colour (black, for example). Check that the camera is on and, on a phone, that DroidCam or the VDO.Ninja page is open.
   - **Picture frozen**: the picture hasn't changed in 10 seconds. On a phone, check whether the screen turned off or the app closed.
3. If "OBS skipped … frames in the last minute" appears, the PC can't keep up. See the last item of the next section.

With a single camera there are no buttons: the card shows only when there is a problem.

## If something goes wrong

- **A DroidCam phone on Wi-Fi gives no picture**: check that the phone and the PC are on the same router and, with 2.4 GHz and 5 GHz networks, on the same one. Check that the "WiFi IP" in the OBS source is the one the phone shows. DroidCam uses port 4747: a firewall on the PC or the router can block it. If nothing works, plug the phone in over USB.
- **A VDO.Ninja phone stays black**: check that the PC and the phone have internet, that the page is still open on the phone and that the screen hasn't locked. If the page closed, scan the QR code again.
- **"No picture" on a camera that is fine, only while it is off air**: in OBS, open the source's properties. For a camera plugged into the PC, turn off **Deactivate when not showing**. For a video or test source, turn off **Restart playback when source becomes active**. With these options on, the source stops when its scene goes off air and the line says "No picture". For VDO.Ninja sources, Set up OBS already leaves the right setting.
- **OBS skipped frames**: the PC can't do it all. Put one camera on 720p instead of 1080p, close other programs and, on a laptop, plug it into power.

## Sources

Read on 26 November 2026.

- DroidCam's quality presets (Mbps per camera), several phones, same network and same band, watermark on HD without Pro: https://droidcam.app/obs/usage/
- DroidCam recommends 720p; USB more stable; charger and heat; port 4747; USB debugging on Android; iPhone over USB: https://droidcam.app/help/
- DroidCam's free version is limited to 480p: https://www.dev47apps.com/droidcam/hd-mode/
- VDO.Ninja, 4000 kbps at 1080p, about 2500 kbps at 720p, audio and overhead on top: https://docs.vdo.ninja/guides/default-resolution-and-bitrate.md
- VDO.Ninja, the `videobitrate` parameter Hallstream uses (4000): https://docs.vdo.ninja/advanced-settings/video-bitrate-parameters/bitrate.md
- VDO.Ninja needs the internet to load the page and connect: https://docs.vdo.ninja/readme/how-does-it-work.md
- VDO.Ninja in OBS, "Shutdown source when not visible" unticked: https://docs.vdo.ninja/guides/enabling-webrtc-sources-in-obs.md
- YouTube, 14 Mbps recommended for 1080p at 30 frames per second (H.264): https://support.google.com/youtube/answer/2853702
