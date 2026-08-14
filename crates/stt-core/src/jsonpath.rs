use serde_json::Value;

pub fn extract_text_from_response(body: &[u8], text_path: &str) -> String {
    let Ok(root) = serde_json::from_slice::<Value>(body) else {
        return String::new();
    };

    if !text_path.is_empty()
        && let Some(value) = extract_by_path(&root, text_path)
    {
        return value;
    }

    let Value::Object(object) = root else {
        return String::new();
    };
    if let Some(value) = object.get("text").and_then(scalar_to_string) {
        return value;
    }
    object
        .values()
        .find_map(|value| match value {
            Value::String(text) if !text.is_empty() => Some(text.clone()),
            _ => None,
        })
        .unwrap_or_default()
}

pub fn extract_by_path(root: &Value, path: &str) -> Option<String> {
    if path.is_empty() {
        return None;
    }

    let mut current = root;
    for token in path.split('.') {
        let (key, indexes) = parse_key_and_indexes(token).ok()?;
        if !key.is_empty() {
            current = current.as_object()?.get(key)?;
        }
        for index in indexes {
            current = current.as_array()?.get(index)?;
        }
    }
    scalar_to_string(current)
}

pub fn parse_key_and_indexes(token: &str) -> Result<(&str, Vec<usize>), String> {
    if token.is_empty() {
        return Err("empty token".into());
    }
    let Some(first_bracket) = token.find('[') else {
        return Ok((token, Vec::new()));
    };

    let key = &token[..first_bracket];
    let mut rest = &token[first_bracket..];
    let mut indexes = Vec::new();
    while !rest.is_empty() {
        if !rest.starts_with('[') {
            return Err(format!("invalid index syntax in {token}"));
        }
        let Some(close) = rest.find(']') else {
            return Err(format!("missing closing ] in {token}"));
        };
        let number = &rest[1..close];
        if number.is_empty() {
            return Err(format!("empty index in {token}"));
        }
        let index = number
            .parse::<usize>()
            .map_err(|_| format!("invalid index '{number}' in {token}"))?;
        indexes.push(index);
        rest = &rest[close + 1..];
    }
    Ok((key, indexes))
}

fn scalar_to_string(value: &Value) -> Option<String> {
    match value {
        Value::String(text) => Some(text.clone()),
        Value::Number(number) => Some(number.to_string()),
        Value::Bool(value) => Some(value.to_string()),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nested_arrays_and_scalar_coercion() {
        let root: Value = serde_json::from_str(
            r#"{"results":[{"alternatives":[{"transcript":true}]}],"number":7.5}"#,
        )
        .unwrap();
        assert_eq!(
            extract_by_path(&root, "results[0].alternatives[0].transcript").as_deref(),
            Some("true")
        );
        assert_eq!(extract_by_path(&root, "number").as_deref(), Some("7.5"));
    }

    #[test]
    fn configured_path_falls_back() {
        assert_eq!(
            extract_text_from_response(br#"{"text":42}"#, "missing.path"),
            "42"
        );
        assert_eq!(
            extract_text_from_response(br#"{"empty":"","other":"value"}"#, ""),
            "value"
        );
    }

    #[test]
    fn invalid_paths_fail_cleanly() {
        let root: Value = serde_json::from_str(r#"{"items":["zero"]}"#).unwrap();
        for path in [
            "",
            "items[-1]",
            "items[bad]",
            "items[]",
            "items[0",
            "items[0]extra",
        ] {
            assert_eq!(extract_by_path(&root, path), None, "path={path}");
        }
    }

    #[test]
    fn parses_multiple_indexes() {
        assert_eq!(
            parse_key_and_indexes("foo[0][1]").unwrap(),
            ("foo", vec![0, 1])
        );
        assert_eq!(parse_key_and_indexes("[2]").unwrap(), ("", vec![2]));
    }
}
