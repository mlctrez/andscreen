package com.andscreen;

import android.app.Activity;
import android.app.Dialog;
import android.content.SharedPreferences;
import android.graphics.Bitmap;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;

import java.util.Calendar;
import android.view.KeyEvent;
import android.view.View;
import android.view.Window;
import android.view.WindowManager;
import android.view.inputmethod.EditorInfo;
import android.view.inputmethod.InputMethodManager;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;

public class MainActivity extends Activity implements ImageScreen.Listener, NetClient.Callbacks {
    private static final String PREFS = "andscreen";
    private static final String KEY_URL = "url";
    private static final long VOLUME_HOLD_MS = 2000;
    private static final int DAY_START_MIN = 9 * 60;
    private static final int DAY_END_MIN = 22 * 60;
    private static final float DAY_BRIGHTNESS = 0.5f;
    private static final float NIGHT_BRIGHTNESS = 0f;

    private TextView statusView;
    private ImageScreen screen;
    private NetClient client;
    private Handler handler;
    private Dialog addressDialog;
    private boolean alive = true;
    private String activeUrl = "";

    private final Runnable openFromVolumeHold = new Runnable() {
        @Override
        public void run() {
            showAddressDialog(false);
        }
    };

    private final Runnable brightnessTick = new Runnable() {
        @Override
        public void run() {
            applyScreenBrightness();
            scheduleBrightness();
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        hideSystemUi();
        setContentView(R.layout.main);
        statusView = (TextView) findViewById(R.id.status);
        screen = (ImageScreen) findViewById(R.id.screen);
        screen.setListener(this);

        handler = new Handler(Looper.getMainLooper());
        brightnessTick.run();
        int longEdge = Math.max(
                getResources().getDisplayMetrics().widthPixels,
                getResources().getDisplayMetrics().heightPixels);
        client = new NetClient(this, handler, Math.max(longEdge, 2048));
        client.start();

        String saved = prefs().getString(KEY_URL, "");
        if (saved.length() > 0) {
            applyUrl(saved);
        } else {
            showAddressDialog(true);
        }
    }

    @Override
    protected void onResume() {
        super.onResume();
        hideSystemUi();
        if (handler != null) {
            brightnessTick.run();
        }
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus) {
            hideSystemUi();
        }
    }

    @Override
    protected void onPause() {
        handler.removeCallbacks(openFromVolumeHold);
        super.onPause();
    }

    @Override
    protected void onDestroy() {
        alive = false;
        handler.removeCallbacks(openFromVolumeHold);
        if (addressDialog != null) {
            addressDialog.dismiss();
            addressDialog = null;
        }
        if (client != null) {
            client.stop();
        }
        handler.removeCallbacksAndMessages(null);
        super.onDestroy();
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent event) {
        if (event.getKeyCode() == KeyEvent.KEYCODE_VOLUME_DOWN) {
            int action = event.getAction();
            if (action == KeyEvent.ACTION_DOWN && event.getRepeatCount() == 0) {
                handler.removeCallbacks(openFromVolumeHold);
                handler.postDelayed(openFromVolumeHold, VOLUME_HOLD_MS);
            } else if (action == KeyEvent.ACTION_UP) {
                handler.removeCallbacks(openFromVolumeHold);
            }
            return true;
        }
        return super.dispatchKeyEvent(event);
    }

    @Override
    public void onImageTouch(TouchEvent event) {
        if (client != null && activeUrl.length() > 0) {
            client.offer(event);
        }
    }

    @Override
    public void onStatus(String text) {
        if (!alive) {
            return;
        }
        if (text == null || text.startsWith("Showing image")) {
            statusView.setVisibility(View.GONE);
            return;
        }
        statusView.setText(text);
        statusView.setVisibility(View.VISIBLE);
    }

    @Override
    public void onBitmap(Bitmap bitmap) {
        if (!alive) {
            if (bitmap != null) {
                bitmap.recycle();
            }
            return;
        }
        screen.setBitmap(bitmap);
        if (bitmap != null) {
            statusView.setVisibility(View.GONE);
        }
    }

    /**
     * @param required first-run setup. The dialog stays until an address is saved.
     *                 A later edit can be dismissed and keeps the address already in use.
     */
    private void showAddressDialog(boolean required) {
        if (addressDialog != null && addressDialog.isShowing()) {
            return;
        }
        final Dialog dialog = new Dialog(this);
        dialog.setContentView(R.layout.dialog_url);
        dialog.setCancelable(!required);
        dialog.setCanceledOnTouchOutside(!required);
        final EditText urlField = (EditText) dialog.findViewById(R.id.url);
        final TextView errorView = (TextView) dialog.findViewById(R.id.dialog_error);
        String current = activeUrl.length() > 0 ? activeUrl : prefs().getString(KEY_URL, "");
        urlField.setText(current);
        urlField.setSelection(urlField.getText().length());
        View.OnClickListener save = new View.OnClickListener() {
            @Override
            public void onClick(View v) {
                saveUrl(dialog, urlField, errorView);
            }
        };
        ((Button) dialog.findViewById(R.id.save)).setOnClickListener(save);
        urlField.setOnEditorActionListener(new TextView.OnEditorActionListener() {
            @Override
            public boolean onEditorAction(TextView view, int actionId, KeyEvent event) {
                if (actionId == EditorInfo.IME_ACTION_DONE) {
                    saveUrl(dialog, urlField, errorView);
                    return true;
                }
                return false;
            }
        });
        dialog.setOnDismissListener(new android.content.DialogInterface.OnDismissListener() {
            @Override
            public void onDismiss(android.content.DialogInterface d) {
                if (addressDialog == dialog) {
                    addressDialog = null;
                }
                hideSystemUi();
            }
        });
        addressDialog = dialog;
        dialog.show();
        Window window = dialog.getWindow();
        if (window != null) {
            int width = (int) (getResources().getDisplayMetrics().widthPixels * 0.55f);
            window.setLayout(width, WindowManager.LayoutParams.WRAP_CONTENT);
            window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        }
        urlField.requestFocus();
        urlField.post(new Runnable() {
            @Override
            public void run() {
                InputMethodManager imm = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
                if (imm != null) {
                    imm.showSoftInput(urlField, InputMethodManager.SHOW_IMPLICIT);
                }
            }
        });
    }

    private void saveUrl(Dialog dialog, EditText urlField, TextView errorView) {
        String normalized = normalize(urlField.getText().toString());
        if (normalized == null) {
            errorView.setText(R.string.url_invalid);
            errorView.setVisibility(View.VISIBLE);
            return;
        }
        urlField.setText(normalized);
        prefs().edit().putString(KEY_URL, normalized).apply();
        InputMethodManager imm = (InputMethodManager) getSystemService(INPUT_METHOD_SERVICE);
        if (imm != null) {
            imm.hideSoftInputFromWindow(urlField.getWindowToken(), 0);
        }
        applyUrl(normalized);
        dialog.dismiss();
    }

    private void applyUrl(String normalized) {
        activeUrl = normalized;
        client.setUrl(normalized);
        onStatus("Connecting to " + normalized);
    }

    private void applyScreenBrightness() {
        WindowManager.LayoutParams attrs = getWindow().getAttributes();
        attrs.screenBrightness = brightnessForMinutes(minutesNow());
        getWindow().setAttributes(attrs);
    }

    private void scheduleBrightness() {
        handler.removeCallbacks(brightnessTick);
        Calendar now = Calendar.getInstance();
        int minutes = minutesNow();
        int target;
        if (minutes < DAY_START_MIN) {
            target = DAY_START_MIN;
        } else if (minutes < DAY_END_MIN) {
            target = DAY_END_MIN;
        } else {
            target = 24 * 60 + DAY_START_MIN;
        }
        long delayMs = (long) (target - minutes) * 60L * 1000L
                - now.get(Calendar.SECOND) * 1000L
                - now.get(Calendar.MILLISECOND);
        if (delayMs < 1L) {
            delayMs = 1L;
        }
        handler.postDelayed(brightnessTick, delayMs);
    }

    private static int minutesNow() {
        Calendar now = Calendar.getInstance();
        return now.get(Calendar.HOUR_OF_DAY) * 60 + now.get(Calendar.MINUTE);
    }

    /**
     * Daytime is 9:00 AM through 10:00 PM. 10:00 PM is night.
     * The window value is a fraction of full brightness.
     */
    static float brightnessForMinutes(int minutesPastMidnight) {
        if (minutesPastMidnight >= DAY_START_MIN && minutesPastMidnight < DAY_END_MIN) {
            return DAY_BRIGHTNESS;
        }
        return NIGHT_BRIGHTNESS;
    }

    private void hideSystemUi() {
        getWindow().getDecorView().setSystemUiVisibility(
                View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                        | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                        | View.SYSTEM_UI_FLAG_FULLSCREEN
                        | View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY);
    }

    private SharedPreferences prefs() {
        return getSharedPreferences(PREFS, MODE_PRIVATE);
    }

    /**
     * Returns a URL with an http or https scheme and a host, or null when the text is not one.
     * A bare host:port is treated as http.
     */
    static String normalize(String raw) {
        if (raw == null) {
            return null;
        }
        String text = raw.trim();
        if (text.length() == 0) {
            return null;
        }
        int scheme = text.indexOf("://");
        if (scheme < 0) {
            text = "http://" + text;
        }
        Uri uri = Uri.parse(text);
        String schemeName = uri.getScheme();
        if (schemeName == null) {
            return null;
        }
        if (!"http".equalsIgnoreCase(schemeName) && !"https".equalsIgnoreCase(schemeName)) {
            return null;
        }
        if (uri.getHost() == null || uri.getHost().length() == 0) {
            return null;
        }
        return text;
    }
}
