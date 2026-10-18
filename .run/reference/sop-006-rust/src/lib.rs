
pub fn greet() -> &'static str {
    "hello"
}

pub fn farewell() -> &'static str {
    "goodbye"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn greet_is_hello() {
        assert_eq!(greet(), "hello");
    }

    #[test]
    fn farewell_is_goodbye() {
        assert_eq!(farewell(), "goodbye");
    }
}
