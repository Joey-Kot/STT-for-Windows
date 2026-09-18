fn main() {
    println!("cargo:rerun-if-changed=../../native/ffmpeg_bridge.c");
    println!("cargo:rerun-if-changed=../../native/ffmpeg_bridge.h");
    println!("cargo:rerun-if-changed=../../assets/icon.ico");

    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() != Ok("windows") {
        return;
    }

    if std::path::Path::new("../../assets/icon.ico").exists() {
        let mut resource = winresource::WindowsResource::new();
        resource.set_icon("../../assets/icon.ico");
        resource
            .compile()
            .expect("failed to embed the Windows application icon");
    }
}
