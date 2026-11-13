fn main() {
    #[cfg(google_cloud_generate_protos)]
    {
        println!("cargo:rerun-if-changed=../proto/gcscore.proto");
        tonic_prost_build::configure()
            .build_client(false)
            .build_server(true)
            .out_dir("src/generated/protos")
            .bytes(".")
            .compile_protos(&["../proto/gcscore.proto"], &["../proto"])
            .expect("compile gcscore.proto");
    }
}
