package com.andscreen;

import android.content.Context;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Color;
import android.graphics.Paint;
import android.graphics.RectF;
import android.util.AttributeSet;
import android.util.SparseBooleanArray;
import android.view.MotionEvent;
import android.view.View;

/**
 * Draws one bitmap, fitted inside the view, and reports touches in that bitmap's coordinate space.
 * x and y are fractions of the drawn image, not of the letterbox bars.
 */
public class ImageScreen extends View {
    interface Listener {
        void onImageTouch(TouchEvent event);
    }

    private final Paint paint = new Paint(Paint.FILTER_BITMAP_FLAG);
    private final RectF dest = new RectF();
    private final SparseBooleanArray tracked = new SparseBooleanArray();
    private Bitmap bitmap;
    private Listener listener;

    public ImageScreen(Context context, AttributeSet attrs) {
        super(context, attrs);
    }

    void setListener(Listener listener) {
        this.listener = listener;
    }

    void setBitmap(Bitmap next) {
        Bitmap previous = bitmap;
        bitmap = next;
        layoutContent();
        invalidate();
        if (previous != null && previous != next) {
            previous.recycle();
        }
    }

    @Override
    protected void onSizeChanged(int w, int h, int oldw, int oldh) {
        layoutContent();
    }

    @Override
    protected void onDraw(Canvas canvas) {
        canvas.drawColor(Color.BLACK);
        layoutContent();
        if (bitmap != null && !bitmap.isRecycled()) {
            canvas.drawBitmap(bitmap, null, dest, paint);
        }
    }

    private void layoutContent() {
        if (bitmap == null || bitmap.isRecycled() || getWidth() == 0 || getHeight() == 0) {
            dest.setEmpty();
            return;
        }
        float scale = Math.min(getWidth() / (float) bitmap.getWidth(), getHeight() / (float) bitmap.getHeight());
        float dw = bitmap.getWidth() * scale;
        float dh = bitmap.getHeight() * scale;
        float left = (getWidth() - dw) / 2f;
        float top = (getHeight() - dh) / 2f;
        dest.set(left, top, left + dw, top + dh);
    }

    @Override
    public boolean onTouchEvent(MotionEvent event) {
        int action = event.getActionMasked();
        int index = event.getActionIndex();
        switch (action) {
            case MotionEvent.ACTION_DOWN:
            case MotionEvent.ACTION_POINTER_DOWN:
                beginPointer(event, index);
                break;
            case MotionEvent.ACTION_MOVE:
                for (int i = 0; i < event.getPointerCount(); i++) {
                    int id = event.getPointerId(i);
                    if (tracked.get(id)) {
                        emit("move", id, event.getX(i), event.getY(i));
                    }
                }
                break;
            case MotionEvent.ACTION_UP:
            case MotionEvent.ACTION_POINTER_UP:
                endPointer(event, index, "up");
                break;
            case MotionEvent.ACTION_CANCEL:
                for (int i = 0; i < tracked.size(); i++) {
                    int id = tracked.keyAt(i);
                    int pi = event.findPointerIndex(id);
                    float x = pi >= 0 ? event.getX(pi) : 0f;
                    float y = pi >= 0 ? event.getY(pi) : 0f;
                    emit("cancel", id, x, y);
                }
                tracked.clear();
                break;
            default:
                break;
        }
        return true;
    }

    private void beginPointer(MotionEvent event, int index) {
        layoutContent();
        float x = event.getX(index);
        float y = event.getY(index);
        if (dest.isEmpty() || !dest.contains(x, y)) {
            return;
        }
        int id = event.getPointerId(index);
        tracked.put(id, true);
        emit("down", id, x, y);
    }

    private void endPointer(MotionEvent event, int index, String action) {
        int id = event.getPointerId(index);
        if (!tracked.get(id)) {
            return;
        }
        tracked.delete(id);
        emit(action, id, event.getX(index), event.getY(index));
    }

    private void emit(String action, int id, float px, float py) {
        layoutContent();
        float nx = 0f;
        float ny = 0f;
        if (dest.width() > 0f && dest.height() > 0f) {
            nx = (px - dest.left) / dest.width();
            ny = (py - dest.top) / dest.height();
        }
        if (listener != null) {
            listener.onImageTouch(new TouchEvent(
                    System.currentTimeMillis(),
                    action,
                    id,
                    nx,
                    ny,
                    px,
                    py,
                    getWidth(),
                    getHeight()));
        }
    }
}
