fn main() {
    println!("cargo:rerun-if-changed=../../native/ffmpeg_bridge.c");
    println!("cargo:rerun-if-changed=../../native/ffmpeg_bridge.h");
    println!("cargo:rerun-if-changed=../../assets/icon.ico");

    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() != Ok("windows") {
        return;
    }

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
            .compiler("x86_64-w64-mingw32-gcc")
            .warnings(false)
            .compile("stt_ffmpeg_bridge");
        for library in libraries {
            pkg_config::Config::new()
                .statik(true)
                .probe(library)
                .unwrap_or_else(|_| panic!("{library} static library is required"));
        }
    }

    if std::path::Path::new("../../assets/icon.ico").exists() {
        let mut resource = winresource::WindowsResource::new();
        resource.set_icon("../../assets/icon.ico");
        resource
            .compile()
            .expect("failed to embed the Windows application icon");
    }
}
