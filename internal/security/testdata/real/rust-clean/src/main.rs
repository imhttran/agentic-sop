// Synthetic clean Rust fixture. It mirrors the seeded fixture with safe
// equivalents and must not match the repository-owned Rust rules.

use std::process::Command;

fn main() {
    let client = reqwest::Client::builder().build();

    let executable = String::from("true");
    Command::new(executable).spawn();
}
