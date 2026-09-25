package com.andscreen;

final class TouchEvent {
    final long tMs;
    final String action;
    final int id;
    final float x;
    final float y;
    final float px;
    final float py;
    final int viewW;
    final int viewH;

    TouchEvent(long tMs, String action, int id, float x, float y, float px, float py, int viewW, int viewH) {
        this.tMs = tMs;
        this.action = action;
        this.id = id;
        this.x = x;
        this.y = y;
        this.px = px;
        this.py = py;
        this.viewW = viewW;
        this.viewH = viewH;
    }
}
