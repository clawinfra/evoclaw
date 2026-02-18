package io.clawinfra.evoclaw

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log

/**
 * BootReceiver — auto-starts the edge agent after device reboot.
 *
 * Triggered by BOOT_COMPLETED and MY_PACKAGE_REPLACED broadcasts.
 * Checks shared prefs for user's auto-start preference before launching.
 */
class BootReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        val action = intent.action
        if (action != Intent.ACTION_BOOT_COMPLETED &&
            action != Intent.ACTION_MY_PACKAGE_REPLACED) return

        Log.i("EvoclawBoot", "Boot received: $action")

        // Respect user preference for auto-start
        val prefs = context.getSharedPreferences("evoclaw", Context.MODE_PRIVATE)
        val autoStart = prefs.getBoolean("auto_start", true)  // default: on

        if (!autoStart) {
            Log.i("EvoclawBoot", "Auto-start disabled by user — skipping")
            return
        }

        Log.i("EvoclawBoot", "Auto-starting EvoClaw agent…")
        val serviceIntent = Intent(context, AgentService::class.java).apply {
            this.action = AgentService.ACTION_START
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            context.startForegroundService(serviceIntent)
        } else {
            context.startService(serviceIntent)
        }
    }
}
