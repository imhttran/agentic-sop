// Synthetic seeded Rust fixture. It is never compiled or executed; each item
// must be detected by the repository-owned Rust rules.

use std::process::Command;

fn main() {
    let client = reqwest::Client::builder()
        .danger_accept_invalid_certs(true)
        .build();

    let executable = std::env::var("SOP_FIXTURE_RUNNER").unwrap();
    Command::new(executable).spawn();

    unsafe {
        libc::getpid();
    }
}
