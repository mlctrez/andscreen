package com.andscreen;

import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.os.Handler;
import android.os.SystemClock;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.ArrayList;
import java.util.Locale;
import java.util.concurrent.ConcurrentLinkedQueue;

/**
 * Polls one URL for an image and posts touch batches to that same URL.
 * The worker thread owns the connection. The UI thread only enqueues events and reads bitmaps.
 */
final class NetClient implements Runnable {
    interface Callbacks {
        void onStatus(String text);

        void onBitmap(Bitmap bitmap);
    }

    private static final int MAX_IMAGE_BYTES = 32 * 1024 * 1024;
    private static final int MAX_EDGE = 2048;
    private static final int QUEUE_LIMIT = 500;

    private final Callbacks callbacks;
    private final Handler ui;
    private final ConcurrentLinkedQueue<TouchEvent> queue = new ConcurrentLinkedQueue<TouchEvent>();
    private final int maxEdge;

    private volatile boolean running = true;
    private volatile String url = "";
    private volatile boolean refresh = true;
    private Thread thread;

    NetClient(Callbacks callbacks, Handler ui, int maxEdge) {
        this.callbacks = callbacks;
        this.ui = ui;
        this.maxEdge = maxEdge > 0 ? maxEdge : MAX_EDGE;
    }

    void start() {
        thread = new Thread(this, "andscreen-net");
        thread.start();
    }

    void stop() {
        running = false;
        Thread current = thread;
        if (current != null) {
            current.interrupt();
        }
    }

    void setUrl(String next) {
        url = next == null ? "" : next;
        refresh = true;
    }

    void offer(TouchEvent event) {
        while (queue.size() > QUEUE_LIMIT) {
            queue.poll();
        }
        queue.offer(event);
    }

    @Override
    public void run() {
        String etag = null;
        long generation = -1;
        long lastGet = 0;
        while (running) {
            if (refresh) {
                refresh = false;
                etag = null;
                generation = -1;
                lastGet = 0;
                queue.clear();
            }
            String current = url;
            if (current.length() == 0) {
                sleep(200);
                continue;
            }
            ArrayList<TouchEvent> batch = new ArrayList<TouchEvent>();
            TouchEvent event;
            while ((event = queue.poll()) != null && batch.size() < 200) {
                batch.add(event);
            }
            try {
                if (!batch.isEmpty()) {
                    long posted = postTouches(current, batch);
                    if (posted >= 0 && posted != generation) {
                        etag = null;
                    }
                }
                long now = SystemClock.elapsedRealtime();
                if (etag == null || now - lastGet >= 1000) {
                    Fetch fetch = fetchImage(current, etag);
                    lastGet = SystemClock.elapsedRealtime();
                    if (fetch.code == 200) {
                        etag = fetch.etag;
                        generation = fetch.generation;
                        postBitmap(fetch.bitmap);
                        postStatus("Showing image " + generationLabel(fetch.generation));
                    } else if (fetch.code == 304) {
                        // The picture on screen is still current.
                    } else if (fetch.code == 404) {
                        etag = null;
                        generation = -1;
                        postBitmap(null);
                        postStatus("Server has no image yet");
                    } else {
                        postStatus("Server returned HTTP " + fetch.code);
                    }
                }
                if (queue.isEmpty()) {
                    sleep(50);
                }
            } catch (InterruptedException interrupted) {
                break;
            } catch (Exception ex) {
                etag = null;
                postStatus("Cannot reach server");
                sleep(1000);
            }
        }
    }

    private long postTouches(String target, ArrayList<TouchEvent> batch) throws Exception {
        byte[] body = encode(batch);
        HttpURLConnection conn = open(target);
        try {
            conn.setRequestMethod("POST");
            conn.setDoOutput(true);
            conn.setFixedLengthStreamingMode(body.length);
            conn.setRequestProperty("Content-Type", "application/json; charset=utf-8");
            OutputStream out = conn.getOutputStream();
            out.write(body);
            out.close();
            int code = conn.getResponseCode();
            String text = readBody(code >= 400 ? conn.getErrorStream() : conn.getInputStream());
            if (code != 200) {
                throw new Exception("touch post HTTP " + code);
            }
            return parseGeneration(text);
        } finally {
            conn.disconnect();
        }
    }

    private Fetch fetchImage(String target, String etag) throws Exception {
        HttpURLConnection conn = open(target);
        try {
            conn.setRequestMethod("GET");
            conn.setRequestProperty("Accept", "image/png, image/jpeg, image/gif, image/webp, */*");
            if (etag != null && etag.length() > 0) {
                conn.setRequestProperty("If-None-Match", etag);
            }
            int code = conn.getResponseCode();
            Fetch fetch = new Fetch();
            fetch.code = code;
            if (code == 304) {
                return fetch;
            }
            if (code != 200) {
                drain(conn.getErrorStream());
                return fetch;
            }
            byte[] bytes = readBytes(conn.getInputStream(), MAX_IMAGE_BYTES);
            fetch.etag = conn.getHeaderField("ETag");
            fetch.generation = generationFromEtag(fetch.etag);
            fetch.bitmap = decode(bytes);
            if (fetch.bitmap == null) {
                throw new Exception("could not decode image");
            }
            return fetch;
        } finally {
            conn.disconnect();
        }
    }

    private HttpURLConnection open(String target) throws Exception {
        HttpURLConnection conn = (HttpURLConnection) new URL(target).openConnection();
        conn.setConnectTimeout(5000);
        conn.setReadTimeout(20000);
        conn.setUseCaches(false);
        conn.setRequestProperty("Cache-Control", "no-cache");
        conn.setRequestProperty("User-Agent", "Andscreen/1.0");
        return conn;
    }

    private Bitmap decode(byte[] bytes) {
        BitmapFactory.Options bounds = new BitmapFactory.Options();
        bounds.inJustDecodeBounds = true;
        BitmapFactory.decodeByteArray(bytes, 0, bytes.length, bounds);
        BitmapFactory.Options opts = new BitmapFactory.Options();
        opts.inSampleSize = sampleSize(bounds.outWidth, bounds.outHeight, maxEdge);
        opts.inPreferredConfig = Bitmap.Config.ARGB_8888;
        return BitmapFactory.decodeByteArray(bytes, 0, bytes.length, opts);
    }

    private void postStatus(final String text) {
        ui.post(new Runnable() {
            @Override
            public void run() {
                callbacks.onStatus(text);
            }
        });
    }

    private void postBitmap(final Bitmap bitmap) {
        ui.post(new Runnable() {
            @Override
            public void run() {
                callbacks.onBitmap(bitmap);
            }
        });
    }

    private static void sleep(long ms) {
        try {
            Thread.sleep(ms);
        } catch (InterruptedException ex) {
            Thread.currentThread().interrupt();
        }
    }

    static int sampleSize(int width, int height, int maxEdge) {
        int sample = 1;
        if (width <= 0 || height <= 0) {
            return sample;
        }
        while (width / sample > maxEdge || height / sample > maxEdge) {
            int next = sample * 2;
            if (next <= sample) {
                break;
            }
            sample = next;
        }
        return sample;
    }

    static byte[] encode(ArrayList<TouchEvent> batch) throws Exception {
        StringBuilder b = new StringBuilder();
        b.append("{\"events\":[");
        for (int i = 0; i < batch.size(); i++) {
            TouchEvent e = batch.get(i);
            if (i > 0) {
                b.append(',');
            }
            b.append("{\"t_ms\":").append(e.tMs);
            b.append(",\"action\":").append(jsonString(e.action));
            b.append(",\"id\":").append(e.id);
            b.append(",\"x\":").append(num(e.x));
            b.append(",\"y\":").append(num(e.y));
            b.append(",\"px\":").append(num(e.px));
            b.append(",\"py\":").append(num(e.py));
            b.append(",\"view_w\":").append(e.viewW);
            b.append(",\"view_h\":").append(e.viewH);
            b.append('}');
        }
        b.append("]}");
        return b.toString().getBytes("UTF-8");
    }

    static long parseGeneration(String body) {
        if (body == null) {
            return -1;
        }
        int key = body.indexOf("\"generation\"");
        if (key < 0) {
            return -1;
        }
        int colon = body.indexOf(':', key);
        if (colon < 0) {
            return -1;
        }
        int j = colon + 1;
        while (j < body.length() && (body.charAt(j) == ' ' || body.charAt(j) == '\n' || body.charAt(j) == '\r' || body.charAt(j) == '\t')) {
            j++;
        }
        int k = j;
        if (k < body.length() && body.charAt(k) == '-') {
            k++;
        }
        int digits = k;
        while (k < body.length() && body.charAt(k) >= '0' && body.charAt(k) <= '9') {
            k++;
        }
        if (k == digits) {
            return -1;
        }
        try {
            return Long.parseLong(body.substring(j, k));
        } catch (NumberFormatException ex) {
            return -1;
        }
    }

    static long generationFromEtag(String etag) {
        if (etag == null) {
            return -1;
        }
        String s = etag.trim();
        if (s.startsWith("W/")) {
            s = s.substring(2).trim();
        }
        if (s.length() >= 2 && s.charAt(0) == '"' && s.charAt(s.length() - 1) == '"') {
            s = s.substring(1, s.length() - 1);
        }
        try {
            return Long.parseLong(s);
        } catch (NumberFormatException ex) {
            return -1;
        }
    }

    private static String generationLabel(long generation) {
        if (generation < 0) {
            return "";
        }
        return "(generation " + generation + ")";
    }

    private static String num(float value) {
        if (Float.isNaN(value) || Float.isInfinite(value)) {
            return "0";
        }
        return String.format(Locale.US, "%.5f", value);
    }

    private static String jsonString(String value) {
        StringBuilder b = new StringBuilder();
        b.append('"');
        for (int i = 0; i < value.length(); i++) {
            char c = value.charAt(i);
            switch (c) {
                case '"':
                    b.append("\\\"");
                    break;
                case '\\':
                    b.append("\\\\");
                    break;
                case '\n':
                    b.append("\\n");
                    break;
                case '\r':
                    b.append("\\r");
                    break;
                case '\t':
                    b.append("\\t");
                    break;
                default:
                    if (c < 0x20) {
                        b.append(String.format(Locale.US, "\\u%04x", Integer.valueOf(c)));
                    } else {
                        b.append(c);
                    }
                    break;
            }
        }
        b.append('"');
        return b.toString();
    }

    private static String readBody(InputStream in) throws Exception {
        if (in == null) {
            return "";
        }
        return new String(readBytes(in, 64 * 1024), "UTF-8");
    }

    private static byte[] readBytes(InputStream in, int limit) throws Exception {
        if (in == null) {
            return new byte[0];
        }
        try {
            ByteArrayOutputStream out = new ByteArrayOutputStream();
            byte[] buf = new byte[8192];
            int total = 0;
            int n;
            while ((n = in.read(buf)) >= 0) {
                total += n;
                if (total > limit) {
                    throw new Exception("response exceeded " + limit + " bytes");
                }
                out.write(buf, 0, n);
            }
            return out.toByteArray();
        } finally {
            in.close();
        }
    }

    private static void drain(InputStream in) {
        if (in == null) {
            return;
        }
        try {
            byte[] buf = new byte[1024];
            while (in.read(buf) >= 0) {
                // discard
            }
        } catch (Exception ignored) {
            // The status code is what the UI needs.
        } finally {
            try {
                in.close();
            } catch (Exception ignored) {
                // already reporting the HTTP status
            }
        }
    }

    private static final class Fetch {
        int code;
        String etag;
        long generation = -1;
        Bitmap bitmap;
    }
}
