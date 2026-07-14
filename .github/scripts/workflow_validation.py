"""Text helpers for workflow policy validators."""


def uncomment_yaml(text: str) -> str:
    lines: list[str] = []
    for line in text.splitlines():
        single_quoted = False
        double_quoted = False
        comment_at: int | None = None
        for index, character in enumerate(line):
            if character == "'" and not double_quoted:
                single_quoted = not single_quoted
            elif (
                character == '"'
                and not single_quoted
                and (
                    len(line[:index]) - len(line[:index].rstrip("\\"))
                )
                % 2
                == 0
            ):
                double_quoted = not double_quoted
            elif (
                character == "#"
                and not single_quoted
                and not double_quoted
                and (index == 0 or line[index - 1].isspace())
            ):
                comment_at = index
                break
        active = line if comment_at is None else line[:comment_at]
        if active.strip():
            lines.append(active.rstrip())
    return "\n".join(lines)


def named_step_body(text: str, step_name: str) -> str | None:
    lines = text.splitlines()
    starts = [
        index
        for index, line in enumerate(lines)
        if line.lstrip() == f"- name: {step_name}"
    ]
    if len(starts) != 1:
        return None
    start = starts[0]
    indent = len(lines[start]) - len(lines[start].lstrip())
    end = len(lines)
    for index in range(start + 1, len(lines)):
        line = lines[index]
        stripped = line.lstrip()
        if (
            len(line) - len(stripped) == indent
            and (stripped == "-" or stripped.startswith("- "))
        ):
            end = index
            break
    return "\n".join(lines[start:end])


def has_only_archive_token(job: str) -> bool:
    secret_lines = [
        line.strip() for line in job.splitlines() if "${{ secrets." in line
    ]
    return secret_lines == [
        "archive-token: ${{ secrets.GITHUB_TOKEN }}"
    ] and "secrets: inherit" not in job


def top_level_grants_actions(text: str) -> bool:
    for line in uncomment_yaml(text).splitlines():
        stripped = line.strip()
        key, separator, value = stripped.partition(":")
        if not separator:
            continue
        normalized_key = key.strip().strip("'\"")
        normalized_value = value.strip().strip("'\"")
        if normalized_key == "actions" or (
            normalized_key == "permissions"
            and normalized_value in {"read-all", "write-all"}
        ):
            return True
    return False
