// Synthetic Rust half of the mixed-language fixture. It is never compiled or
// executed.

pub fn client() -> reqwest::Client {
    reqwest::Client::builder()
        .danger_accept_invalid_hostnames(true)
        .build()
}
