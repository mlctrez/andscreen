# Andscreen

Andscreen shows a picture on a Nexus 7 and sends touches on that picture to a server on the same Wi-Fi network. The tablet and the server share one URL: `GET` returns the current image, and `POST` sends touch events. USB is only for installing the app.

The tablet is a 2013 Nexus 7 (`flo`), Android 6.0.1, API 23, 1920×1200 in landscape. The app is a small Java program, package `com.andscreen`, built without Gradle from the Ubuntu universe Android packages (`android-sdk`, `android-sdk-platform-23`, `dalvik-exchange`, `openjdk-17-jdk`). The server is the Go 1.25 module `github.com/mlctrez/andscreen`. It draws the frame offscreen with `github.com/gogpu/ui` v0.1.54 and reads Fahrenheit weather from `github.com/briandowns/openweathermap` v0.21.1.

## Tablet

The picture fills the screen. The status and navigation bars stay hidden. From 9:00 AM until 10:00 PM the screen is at half brightness. At 10:00 PM it drops to 0, and it returns to half brightness at 9:00 AM. The app applies that on its own while it is open.

The server address is not on screen during use. The first launch asks for it and will not continue until one is saved. After that, hold volume-down for about two seconds to open the same dialog. That key is consumed, so the volume stays where it is. Save stores the address and closes the dialog. Back, or a tap outside the dialog, keeps the address already in use. The address is remembered across launches. A bare `host:port` is stored as `http://host:port`.

While a picture is showing, the only extra text is a failure line along the bottom. If the server cannot be reached, that line reads `Cannot reach server` and goes away when an image loads again. `Server has no image yet` is shown when `GET` returns 404.

Touches are reported in the coordinate space of the drawn image. `x` and `y` are fractions of that image, so letterbox bars are not part of the range. A finger that slides off the image can report values outside 0–1. `px` and `py` are pixels in the view.

A vertical seam can still appear for a frame when the picture changes. `ImageScreen` is a normal `View` that draws the bitmap in `onDraw` and recycles the previous bitmap immediately.

## Image updates

The server numbers each served image with a generation counter. `GET` sends that number as an `ETag`, a quoted integer such as `"3"`.

When the tablet is idle and already showing an image, it asks for the image about once a second. The request includes `If-None-Match` with the `ETag` it already has. A `304` means the picture is unchanged and the screen is left alone. A `200` means the generation moved on, and the new body replaces the picture. The one-second check is measured from the previous `GET`, and the network loop wakes about every 50 ms, so the poll lands on about that one-second mark rather than on a separate timer.

A touch `POST` is answered with JSON:

```json
{"generation": 3}
```

The tablet compares that number with the generation of the picture on screen.

*   If the number is different, the tablet fetches immediately. It does not wait for the next idle poll. The fetch is the next step in the same loop, as soon as the `POST` response has been read.
*   If the number is the same, the picture on screen is still the one the server is serving. The tablet keeps it and goes back to the one-second poll.

That is how a touch that takes time to produce an image still updates the device when the image is actually finished.

The server can answer the `POST` at once with the generation currently on screen, and do the work afterward. The tablet keeps the old picture. When the finished image replaces the one being served, the server increments the generation. The next idle `GET` sees the new `ETag` and draws the result, so the screen updates within about a second of the image becoming ready.

The `POST` can instead wait until the new image is the one `GET` will return, and only then answer with that new generation. The tablet fetches as soon as it reads the response. It does not wait for the idle poll.

One background thread does all of the network work. It does not start a `GET` while a `POST` is still open, so the one-second poll does not run during the wait and cannot pull a half-finished image over that request. A render that takes several seconds just pauses the poll. Touches that arrive during the wait are kept, as described under Touch request. If the response generation is new, the tablet loads that image before it sends the waiting touches.

The tablet stops reading a response after 20 seconds. If the `POST` is held longer than that, the bottom line reads `Cannot reach server` and the tablet tries again about a second later. If the generation did advance before the timeout, that retry still loads the finished image.

Generation must change only when the bytes returned by `GET` are the image that should be shown. A `POST` that returns a new generation before those bytes are ready makes the tablet fetch the old picture again.

## Touch request

`POST` body:

```json
{
  "events": [
    {
      "t_ms": 10,
      "action": "down",
      "id": 0,
      "x": 0.5,
      "y": 0.25,
      "px": 960,
      "py": 300,
      "view_w": 1920,
      "view_h": 1200
    }
  ]
}
```

`action` is `down`, `move`, `up`, or `cancel`. `id` is the pointer. `down` is the press. Moves are delivered too, so a server can follow a drag.

Touches that happen while a request is in flight are queued. They are not dropped on a normal burst of downs, moves, and ups. The network thread takes up to 200 waiting events into the `POST` it is sending. Anything that arrives after that stays in the queue. When the in-flight `POST` returns, the thread finishes the image fetch if the generation changed or the one-second poll is due, then sends the waiting events in later requests of up to 200.

The queue keeps the newest events if input arrives faster than it can be sent. Once more than 500 events are waiting, each new event drops the oldest waiting one. Events already copied into the `POST` that is on the wire are not put back if that request fails. Events that were still waiting are kept and sent after the one-second pause that follows `Cannot reach server`. Saving a new server address clears the queue.

## This server

The server serves one 1920×1200 time and temperature screen. There are no image-file arguments.

```bash
cd server
go run ./cmd/andscreen
```

The listen address is `LISTEN`, or `:8080` when that variable is empty. The clock, weekday, and date are the machine's local time. The frame is redrawn at the start of each minute, and that redraw advances the generation. The tablet's idle poll picks the new frame up.

The layout, in `server/internal/app/timetemp.go`, is dark Material:

*   Upper left: the time at 168px, with a smaller AM/PM beside it.
*   Upper right: the weekday at 88px, then the date at 64px.
*   Large figure on the left: the current temperature at 168px.
*   To its right, smaller and on one line: today's high and today's low at 64px. An 83px spacer drops that pair toward the baseline of the current temperature. In this version of `gogpu/ui`, an `HBox` ignores cross-axis alignment.
*   Under that: the condition word only, such as `Clouds` or `Clear`, at 72px.
*   Heading `5 day forecast` at 64px.
*   Five day tiles. The weekday is 54px. Each tile has a high, a low, and a condition.

`DrawText` draws one line and does not clip, so each string has to fit its column. `primitives.Image` paints a placeholder. The sample screens in `server/internal/app/screens.go` draw a real bitmap with `placedImage`. The time screen does not use that widget.

Until a weather fetch succeeds, those readings are an em dash. Today's high is the max of the current temperature and today's forecast `temp_max`. Today's low is the min of the current temperature and today's `temp_min`. Each later day's condition is the forecast slot whose hour is closest to 3 PM local. A missing condition stays an em dash.

Weather uses `OWM_KEY` and `OWM_ZIP`. For development, `github.com/joho/godotenv` loads a `.env` file from the working directory. Variables already set in the process stay as they are. A missing `.env` is fine, and `server/.env` is gitignored. `server/.env.sample` lists the variables. The key is not written to the log.

A fetch runs on the half hour from 6:00 AM through 11:00 PM inclusive. Outside that window, and between those half hours, the last reading stays on screen. A failed fetch is tried again at the next half hour. The 5-day API only has slots from now forward, so late in the day today's high can miss a peak that already happened.

A press is logged with the pointer and where it landed on the image. The time screen stays up.

`go test ./...` covers the HTTP generation rules, the minute changing the image, the 6:00–23:00 window, and forecast aggregation. It does not call OpenWeatherMap. `go test -tags live -run TestLiveWeatherPreview` does call the API and writes `server/time-temp.png`.

The command is `server/cmd/andscreen`. The screen, the HTTP hub, the weather fetch, and the service lifecycle live in `server/internal/app`.

## Service

The process runs under [`github.com/mlctrez/servicego`](https://github.com/mlctrez/servicego). `cmd/andscreen` calls `servicego.Run`. `servicego` parses `-action`.

`Start` loads a working-directory `.env` when one is present, draws the first frame, binds the listen address, and returns while the HTTP server and the minute loop run. `Stop` cancels that loop and shuts the listener down.

With no arguments the process runs in the foreground, the same as `-action run`. `-action` also accepts `install`, `uninstall`, `start`, `stop`, `restart`, and `deploy`.

The listen address is `LISTEN` from the environment or `.env`. An empty value binds `:8080`.

An installed unit uses working directory `/opt/servicego/<executable name>` and reads `/etc/sysconfig/<executable name>`. Build the binary as `andscreen` so the unit, the directory, and the sysconfig file share that name. `OWM_KEY`, `OWM_ZIP`, and `LISTEN` belong in that file. Run `go run` from `server/` when the development `.env` should load.

```bash
cd server
go build -o andscreen ./cmd/andscreen
```

## Build and install

From the repo root, `make` builds the APK and the server binary. `make android` and `make server` build one of them. `make deploy-apk` installs the APK on the attached Nexus 7 and starts the app again. `make deploy` still copies the server to dune and installs it. The server binary is written to `/tmp/andscreen`.

```bash
make
make deploy-apk
cd server && go test ./...
cd server && go run ./cmd/andscreen
```

The debug key is `android/debug.keystore`, so a later install replaces this app. The APK is signed with signature scheme v1, which Android 6 requires. Sources stay free of lambdas and method references so the output is Java 8 bytecode.

## Layout

| Path | Role |
|---|---|
| `Makefile` | Builds the APK and the server. `make deploy-apk` installs and starts the app. `make deploy` installs the server on dune |
| `android/build.sh` | `aapt`, `javac --release 8`, `dalvik-exchange`, `zipalign`, `apksigner` |
| `android/src/com/andscreen/MainActivity.java` | Fullscreen activity, address dialog, volume-down hold, brightness |
| `android/src/com/andscreen/ImageScreen.java` | Bitmap view and touch mapping |
| `android/src/com/andscreen/NetClient.java` | GET/POST loop, queue, 20s timeout |
| `server/cmd/andscreen/main.go` | Hands the process to `servicego` |
| `server/internal/app/service.go` | Start/stop, HTTP server, minute loop |
| `server/internal/app/hub.go` | HTTP, generation, press log |
| `server/internal/app/timetemp.go` | Layout and the minute / weather schedule |
| `server/internal/app/weather.go` | OpenWeatherMap fetch and daily high/low |
| `server/internal/app/screens.go` | Older sample screens plus `placedImage` |
| `server/.env.sample` | `OWM_KEY`, `OWM_ZIP`, and `LISTEN` |
