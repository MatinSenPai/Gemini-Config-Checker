package app.geminicheck

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import java.io.File

/**
 * Runs the Go engine (shipped as libgccserver.so, an executable) as a child process and keeps it alive in the
 * foreground, so a long scan survives the screen turning off. The UI is the same web page the desktop build serves.
 */
class ServerService : Service() {
    companion object {
        const val PORT = 18787
        private const val CHANNEL = "engine"
        private const val NOTIFICATION_ID = 1
        private const val ACTION_STOP = "app.geminicheck.STOP"
        private const val TAG = "GeminiCheck"
    }

    private var engine: Process? = null

    override fun onCreate() {
        super.onCreate()
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL, getString(R.string.channel_name), NotificationManager.IMPORTANCE_LOW))

        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val stop = PendingIntent.getService(
            this, 1, Intent(this, ServerService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = Notification.Builder(this, CHANNEL)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(getString(R.string.notification_text))
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setContentIntent(open)
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, getString(R.string.stop), stop).build())
            .build()
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
        startEngine()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }
        if (engine?.isAlive != true) startEngine()
        return START_STICKY
    }

    private fun startEngine() {
        val binary = File(applicationInfo.nativeLibraryDir, "libgccserver.so")
        if (!binary.exists()) {
            Log.e(TAG, "engine binary missing: $binary")
            return
        }
        try {
            val builder = ProcessBuilder(binary.path, "-addr", "127.0.0.1:$PORT", "-no-open").redirectErrorStream(true)
            builder.environment()["HOME"] = filesDir.path // the engine keeps its settings under $HOME
            builder.environment()["TMPDIR"] = cacheDir.path
            val process = builder.start()
            engine = process
            Thread { process.inputStream.bufferedReader().forEachLine { Log.i(TAG, it) } }.start()
        } catch (e: Exception) {
            Log.e(TAG, "cannot start the engine", e)
        }
    }

    override fun onDestroy() {
        engine?.destroy()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null
}
