package com.echomuse.crownlauncher;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.media.AudioAttributes;
import android.media.AudioFormat;
import android.media.AudioManager;
import android.media.AudioTrack;
import android.util.Log;

/**
 * Throwaway diagnostic — NOT part of the shipped design, exists only to
 * answer one question before writing any real socket/service code: does
 * this SoC/ROM actually grant AudioTrack's low-latency ("fast mixer") path,
 * or does it silently fall back to the shared/legacy mixer?
 *
 * See docs/echo-show-8-audiotrack-design.md ("Q2"). Trigger with:
 *   adb shell am broadcast -a com.echomuse.crownlauncher.PROBE_AUDIO \
 *       -n com.echomuse.crownlauncher/.AudioProbeReceiver
 * Read the result with:
 *   adb logcat -s EchoMuseProbe
 *
 * Uses the exact format the real design would use (48kHz stereo S16LE,
 * USAGE_ASSISTANT/CONTENT_TYPE_SPEECH) so the answer is about the actual
 * proposed path, not a generic AudioTrack default.
 */
public class AudioProbeReceiver extends BroadcastReceiver {
    private static final String TAG = "EchoMuseProbe";
    private static final int SAMPLE_RATE = 48000;

    @Override
    public void onReceive(Context context, Intent intent) {
        Log.i(TAG, "=== probe start ===");

        AudioManager am = context.getSystemService(AudioManager.class);
        if (am != null) {
            Log.i(TAG, "AudioManager.PROPERTY_OUTPUT_SAMPLE_RATE=" +
                    am.getProperty(AudioManager.PROPERTY_OUTPUT_SAMPLE_RATE));
            Log.i(TAG, "AudioManager.PROPERTY_OUTPUT_FRAMES_PER_BUFFER=" +
                    am.getProperty(AudioManager.PROPERTY_OUTPUT_FRAMES_PER_BUFFER));
        } else {
            Log.w(TAG, "AudioManager unavailable");
        }

        AudioAttributes attrs = new AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_ASSISTANT)
                .setContentType(AudioAttributes.CONTENT_TYPE_SPEECH)
                .build();

        AudioFormat format = new AudioFormat.Builder()
                .setSampleRate(SAMPLE_RATE)
                .setChannelMask(AudioFormat.CHANNEL_OUT_STEREO)
                .setEncoding(AudioFormat.ENCODING_PCM_16BIT)
                .build();

        int minBuf = AudioTrack.getMinBufferSize(SAMPLE_RATE,
                AudioFormat.CHANNEL_OUT_STEREO, AudioFormat.ENCODING_PCM_16BIT);
        Log.i(TAG, "AudioTrack.getMinBufferSize=" + minBuf + " bytes");

        AudioTrack track;
        try {
            track = new AudioTrack.Builder()
                    .setAudioAttributes(attrs)
                    .setAudioFormat(format)
                    .setBufferSizeInBytes(Math.max(minBuf, 4096))
                    .setTransferMode(AudioTrack.MODE_STREAM)
                    .setPerformanceMode(AudioTrack.PERFORMANCE_MODE_LOW_LATENCY)
                    .build();
        } catch (Exception e) {
            Log.e(TAG, "AudioTrack construction failed: " + e);
            Log.i(TAG, "=== probe end (construction failed) ===");
            return;
        }

        int granted = track.getPerformanceMode();
        String grantedStr = granted == AudioTrack.PERFORMANCE_MODE_LOW_LATENCY ? "LOW_LATENCY (granted as requested)"
                : granted == AudioTrack.PERFORMANCE_MODE_POWER_SAVING ? "POWER_SAVING (NOT what we asked for)"
                : granted == AudioTrack.PERFORMANCE_MODE_NONE ? "NONE — fell back to legacy/shared mixer path"
                : "unknown(" + granted + ")";
        Log.i(TAG, "requested PERFORMANCE_MODE_LOW_LATENCY, granted: " + grantedStr);
        Log.i(TAG, "getBufferSizeInFrames=" + track.getBufferSizeInFrames());
        Log.i(TAG, "getSampleRate=" + track.getSampleRate());
        Log.i(TAG, "getState=" + track.getState() + " (1=INITIALIZED expected)");

        // Play ~1s of a 440Hz tone so a human can also confirm audibly that
        // output actually happens on this build, not just that it constructs.
        short[] tone = generateTone(SAMPLE_RATE, 440.0, 1.0);
        track.play();
        int written = track.write(tone, 0, tone.length);
        Log.i(TAG, "wrote " + written + " of " + tone.length + " samples");
        track.stop();
        track.release();

        Log.i(TAG, "=== probe end ===");
    }

    private static short[] generateTone(int sampleRate, double freqHz, double seconds) {
        int frames = (int) (sampleRate * seconds);
        short[] out = new short[frames * 2]; // stereo interleaved
        for (int i = 0; i < frames; i++) {
            short sample = (short) (Math.sin(2 * Math.PI * freqHz * i / sampleRate) * 12000);
            out[i * 2] = sample;
            out[i * 2 + 1] = sample;
        }
        return out;
    }
}
