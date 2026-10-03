plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Release metadata and signing come from the environment (set by the release workflow).
// Without a keystore the build falls back to the debug key, so local builds still install.
val versionNameEnv = System.getenv("VERSION_NAME") ?: "0.1.0"
val versionCodeEnv = (System.getenv("VERSION_CODE") ?: "1").toInt()
val keystorePath: String? = System.getenv("KEYSTORE_PATH")

android {
    namespace = "app.geminicheck"
    compileSdk = 35

    defaultConfig {
        applicationId = "app.geminicheck"
        minSdk = 26
        targetSdk = 34
        versionCode = versionCodeEnv
        versionName = versionNameEnv
        // The Go engine ships for 64-bit ARM only (every current phone, and Apple-silicon emulators).
        ndk { abiFilters += "arm64-v8a" }
    }

    signingConfigs {
        if (keystorePath != null) {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = System.getenv("KEYSTORE_PASSWORD")
                keyAlias = System.getenv("KEY_ALIAS")
                keyPassword = System.getenv("KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = if (keystorePath != null) signingConfigs.getByName("release") else signingConfigs.getByName("debug")
        }
    }

    // The engine is an executable named lib*.so inside jniLibs; Android extracts it so the service can run it.
    packaging {
        jniLibs { useLegacyPackaging = true }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}
