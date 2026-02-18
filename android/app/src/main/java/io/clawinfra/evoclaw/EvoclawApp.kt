package io.clawinfra.evoclaw

import android.app.Application
import android.util.Log

class EvoclawApp : Application() {
    override fun onCreate() {
        super.onCreate()
        Log.i("EvoclawApp", "EvoClaw v${BuildConfig.VERSION_NAME} starting")
    }
}
