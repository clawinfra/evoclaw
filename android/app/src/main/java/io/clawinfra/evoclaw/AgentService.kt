package io.clawinfra.evoclaw

import android.app.*
import android.content.Intent
import android.os.*
import android.util.Log
import androidx.core.app.NotificationCompat
import java.io.File
import java.io.FileOutputStream

/**
 * AgentService — Foreground service that runs the EvoClaw native edge agent.
 *
 * The native binary (evoclaw-agent) is bundled in jniLibs/ for the device ABI,
 * extracted to the app's files directory on first run, then executed as a
 * child process. Stdout/stderr are captured to a rolling log file.
 *
 * Lifecycle:
 *   START_SERVICE → extract binary → start process → foreground notification
 *   STOP_SERVICE  → kill process   → stop foreground → stopSelf
 */
class AgentService : Service() {

    companion object {
        private const val TAG = "EvoclawAgent"
        private const val NOTIFICATION_ID = 1001
        private const val CHANNEL_ID = "evoclaw_agent"
        private const val MAX_LOG_BYTES = 5 * 1024 * 1024L  // 5MB rolling log

        const val ACTION_START = "io.clawinfra.evoclaw.ACTION_START"
        const val ACTION_STOP  = "io.clawinfra.evoclaw.ACTION_STOP"
        const val ACTION_STATUS = "io.clawinfra.evoclaw.ACTION_STATUS"

        // Broadcast intent sent by service with status updates
        const val BROADCAST_STATUS = "io.clawinfra.evoclaw.AGENT_STATUS"
        const val EXTRA_STATUS = "status"
        const val EXTRA_PID = "pid"
    }

    private var agentProcess: Process? = null
    private var logThread: Thread? = null
    private var wakeLock: PowerManager.WakeLock? = null

    // ── Service Lifecycle ──────────────────────────────────────────────────

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        Log.i(TAG, "AgentService created")
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                stopAgent()
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
                return START_NOT_STICKY
            }
            else -> startAgent()
        }
        return START_STICKY  // Restart if killed by system
    }

    override fun onDestroy() {
        super.onDestroy()
        stopAgent()
        Log.i(TAG, "AgentService destroyed")
    }

    override fun onBind(intent: Intent?): IBinder? = null

    // ── Agent Lifecycle ────────────────────────────────────────────────────

    private fun startAgent() {
        if (agentProcess?.isAlive == true) {
            Log.i(TAG, "Agent already running (PID ${agentProcess?.pid()})")
            broadcastStatus("running", agentProcess?.pid() ?: -1)
            return
        }

        Log.i(TAG, "Starting EvoClaw edge agent...")

        // Show foreground notification immediately (required before long operations)
        startForeground(NOTIFICATION_ID, buildNotification("Starting…"))

        // Acquire partial wake lock to prevent CPU sleep
        val pm = getSystemService(POWER_SERVICE) as PowerManager
        wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "$TAG:WakeLock").apply {
            acquire(24 * 60 * 60 * 1000L)  // 24h, renewed by START_STICKY
        }

        try {
            val binary = extractBinary()
            val configFile = ensureConfig()
            val logFile = File(filesDir, "logs/agent.log")
            logFile.parentFile?.mkdirs()

            val process = ProcessBuilder(
                binary.absolutePath,
                "--config", configFile.absolutePath
            )
                .directory(filesDir)
                .redirectErrorStream(true)
                .start()

            agentProcess = process
            val pid = process.pid()
            Log.i(TAG, "Agent started (PID $pid)")

            // Capture logs in background thread
            logThread = Thread({
                captureOutput(process, logFile)
            }, "agent-log").apply { isDaemon = true; start() }

            updateNotification("Running (PID $pid)")
            broadcastStatus("running", pid)

        } catch (e: Exception) {
            Log.e(TAG, "Failed to start agent", e)
            updateNotification("Error: ${e.message}")
            broadcastStatus("error", -1)
        }
    }

    private fun stopAgent() {
        agentProcess?.let { proc ->
            Log.i(TAG, "Stopping agent (PID ${proc.pid()})…")
            proc.destroy()
            try { proc.waitFor(5, java.util.concurrent.TimeUnit.SECONDS) }
            catch (_: InterruptedException) {}
            if (proc.isAlive) proc.destroyForcibly()
            Log.i(TAG, "Agent stopped")
        }
        agentProcess = null
        logThread = null
        wakeLock?.release()
        wakeLock = null
        broadcastStatus("stopped", -1)
    }

    // ── Binary Extraction ──────────────────────────────────────────────────

    /**
     * Extract the bundled native binary from jniLibs to app filesDir.
     * The binary is stored as libevoclaw_agent.so (Android jniLibs convention)
     * but executed directly — not loaded as a shared library.
     */
    private fun extractBinary(): File {
        val destFile = File(filesDir, "bin/evoclaw-agent")
        destFile.parentFile?.mkdirs()

        // Always re-extract on version bump (compare file size as proxy)
        val sourceLib = File(applicationInfo.nativeLibraryDir, "libevoclaw_agent.so")
        if (!destFile.exists() || destFile.length() != sourceLib.length()) {
            Log.i(TAG, "Extracting binary from ${sourceLib.absolutePath}…")
            sourceLib.inputStream().use { src ->
                FileOutputStream(destFile).use { dst -> src.copyTo(dst) }
            }
            destFile.setExecutable(true)
            Log.i(TAG, "Binary extracted: ${destFile.length()} bytes")
        }
        return destFile
    }

    /**
     * Create a default agent.toml if none exists.
     */
    private fun ensureConfig(): File {
        val configFile = File(filesDir, "agent.toml")
        if (!configFile.exists()) {
            val deviceModel = Build.MODEL.replace(" ", "-").lowercase()
            configFile.writeText("""
                [agent]
                agent_id   = "$deviceModel-android"
                agent_type = "monitor"
                capabilities = "Android device — battery, storage, network, camera"

                [orchestrator]
                # Edit this via the EvoClaw app settings
                mqtt_broker = "REPLACE_WITH_ORCHESTRATOR_IP"
                mqtt_port   = 1883

                [llm]
                base_url = "https://api.anthropic.com"
                api_key  = "REPLACE_WITH_API_KEY"
                model    = "glm-4.7"
            """.trimIndent())
            Log.i(TAG, "Default config created at ${configFile.absolutePath}")
        }
        return configFile
    }

    // ── Log Capture ────────────────────────────────────────────────────────

    private fun captureOutput(process: Process, logFile: File) {
        try {
            process.inputStream.bufferedReader().forEachLine { line ->
                Log.d(TAG, line)
                // Rolling log: truncate if > 5MB
                if (logFile.length() < MAX_LOG_BYTES) {
                    logFile.appendText("$line\n")
                } else {
                    // Rotate: keep last 2MB
                    val content = logFile.readText()
                    logFile.writeText(content.takeLast((2 * 1024 * 1024).toInt()))
                    logFile.appendText("$line\n")
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Log capture error", e)
        }
        Log.i(TAG, "Agent process exited (code ${runCatching { process.exitValue() }.getOrDefault(-1)})")
        broadcastStatus("stopped", -1)
        updateNotification("Stopped")
    }

    // ── Notifications ──────────────────────────────────────────────────────

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "EvoClaw Agent",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "Persistent notification for the running edge agent"
                setShowBadge(false)
            }
            (getSystemService(NOTIFICATION_SERVICE) as NotificationManager)
                .createNotificationChannel(channel)
        }
    }

    private fun buildNotification(status: String): Notification {
        val stopIntent = Intent(this, AgentService::class.java).apply { action = ACTION_STOP }
        val stopPi = PendingIntent.getService(
            this, 0, stopIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val mainIntent = Intent(this, MainActivity::class.java)
        val mainPi = PendingIntent.getActivity(
            this, 0, mainIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("🦞 EvoClaw Agent")
            .setContentText(status)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentIntent(mainPi)
            .addAction(android.R.drawable.ic_delete, "Stop", stopPi)
            .setOngoing(true)
            .setSilent(true)
            .build()
    }

    private fun updateNotification(status: String) {
        val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        nm.notify(NOTIFICATION_ID, buildNotification(status))
    }

    private fun broadcastStatus(status: String, pid: Int) {
        sendBroadcast(Intent(BROADCAST_STATUS).apply {
            putExtra(EXTRA_STATUS, status)
            putExtra(EXTRA_PID, pid)
        })
    }

    private fun Process.pid(): Int = pid()
    private val Process.isAlive: Boolean get() = runCatching { exitValue(); false }.getOrDefault(true)
}
