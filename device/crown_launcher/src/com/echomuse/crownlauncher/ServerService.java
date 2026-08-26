package com.echomuse.crownlauncher;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;

import java.io.File;
import java.io.IOException;

/**
 * Runs the real EchoMuse binary as a child process, as a foreground
 * service so Android doesn't kill it as background work.
 *
 * Deliberately as minimal as echomuse_crown.rc: exec the binary, log to
 * the same /data/local/tmp/echomuse.log path provision_crown.sh already
 * tails, START_STICKY so Android restarts the service (and therefore this
 * process) if it's killed. No log rotation, no supervisor, no A/B slots —
 * same reasoning as the .rc file: one dev unit, no fleet yet to need it.
 */
public class ServerService extends Service {
    private static final String CHANNEL_ID = "echomuse_crown";
    private static final String BINARY = "/data/local/bin/server";
    private static final String LOG_FILE = "/data/local/tmp/echomuse.log";

    private Process proc;
    private PlaybackServer playbackServer;
    private Thread playbackThread;

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        startForegroundCompat();
        // Start the playback socket server before the daemon, so the
        // socket file exists by the time the daemon's first connect
        // attempt happens — see docs/echo-show-8-audiotrack-design.md.
        playbackServer = new PlaybackServer();
        playbackThread = new Thread(playbackServer, "echomuse-playback");
        playbackThread.start();
        try {
            proc = new ProcessBuilder(BINARY)
                    .redirectErrorStream(true)
                    .redirectOutput(new File(LOG_FILE))
                    .start();
        } catch (IOException e) {
            stopSelf();
            return START_NOT_STICKY;
        }
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        if (proc != null) {
            proc.destroy();
        }
        if (playbackServer != null) {
            playbackServer.stop();
        }
        super.onDestroy();
    }

    private void startForegroundCompat() {
        NotificationManager nm = getSystemService(NotificationManager.class);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            nm.createNotificationChannel(new NotificationChannel(
                    CHANNEL_ID, "EchoMuse", NotificationManager.IMPORTANCE_MIN));
        }
        Notification notification = new Notification.Builder(this, CHANNEL_ID)
                .setContentTitle("EchoMuse")
                .setContentText("running")
                .setSmallIcon(android.R.drawable.ic_media_play)
                .build();
        startForeground(1, notification);
    }
}
