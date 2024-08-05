use std::{env, path::PathBuf, str::FromStr};

fn main() {
    let crate_dir = env::var("CARGO_MANIFEST_DIR").unwrap();
    let create_name = env::var("CARGO_PKG_NAME").unwrap();
    let write_dest = PathBuf::from_str(crate_dir.as_str())
        .unwrap()
        .join("..")
        .join("..")
        .join("target")
        .join("release")
        .join(format!("lib{}.h", create_name));

    let mut conf = cbindgen::Config::default();
    conf.no_includes = true;

    cbindgen::Builder::new()
        .with_crate(crate_dir)
        .with_config(conf)
        .generate()
        .expect("Unable to generate bindings")
        .write_to_file(write_dest);
}
