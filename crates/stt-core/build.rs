fn main() {
    println!("cargo:rerun-if-changed=../../native/ffmpeg_bridge.c");
    println!("cargo:rerun-if-changed=../../native/ffmpeg_bridge.h");
    if std::env::var_os("CARGO_FEATURE_STATIC_LIBAV").is_some() {
        let libraries = ["libavformat", "libavcodec", "libswresample", "libavutil"];
        let mut include_paths = Vec::new();
        for library in libraries {
            let found = pkg_config::Config::new()
                .statik(true)
                .cargo_metadata(false)
                .probe(library)
                .unwrap_or_else(|_| panic!("{library} static library is required"));
            include_paths.extend(found.include_paths);
        }
        let mut build = cc::Build::new();
        build
            .file("../../native/ffmpeg_bridge.c")
            .includes(include_paths)
            .warnings(false)
            .compile("stt_ffmpeg_bridge");
        for library in libraries {
            pkg_config::Config::new()
                .statik(true)
                .probe(library)
                .unwrap_or_else(|_| panic!("{library} static library is required"));
        }
    }

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
