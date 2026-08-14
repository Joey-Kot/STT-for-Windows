fn main() {
    println!("cargo:rerun-if-env-changed=PKG_CONFIG_PATH");
    println!("cargo:rerun-if-env-changed=STT_REQUIRE_PORTAUDIO");

    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() != Ok("windows") {
        return;
    }

    // `cargo check --target` must remain usable without native archives. Release
    // builds set STT_REQUIRE_PORTAUDIO and provide the MinGW pkg-config file.
    if std::env::var_os("STT_REQUIRE_PORTAUDIO").is_some() {
        pkg_config::Config::new()
            .statik(true)
            .probe("portaudio-2.0")
            .expect("PortAudio static library is required for Windows release builds");
    }
}
