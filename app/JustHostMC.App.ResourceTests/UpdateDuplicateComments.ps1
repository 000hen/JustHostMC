[CmdletBinding()]
param(
    [string]$RepositoryRoot
)

if ([string]::IsNullOrWhiteSpace($RepositoryRoot)) {
    $RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
}

$resourceFiles = @(
    Join-Path $RepositoryRoot "app\JustHostMC.App\Strings\en-US\Resources.resw"
    Join-Path $RepositoryRoot "app\JustHostMC.App\Strings\zh-TW\Resources.resw"
)

foreach ($path in $resourceFiles) {
    $document = [System.Xml.XmlDocument]::new()
    $document.PreserveWhitespace = $true
    $document.Load($path)

    $entries = @($document.DocumentElement.SelectNodes("data"))
    $groups = $entries |
        Group-Object { $_.SelectSingleNode("value").InnerText } |
        Where-Object Count -gt 1

    foreach ($group in $groups) {
        $canonical = $group.Group[0].GetAttribute("name")
        foreach ($entry in $group.Group) {
            if ($null -ne $entry.SelectSingleNode("comment")) {
                continue
            }

            $name = $entry.GetAttribute("name")
            $comment = $document.CreateElement("comment")
            $comment.InnerText = if ($name -eq $canonical) {
                "Canonical duplicate value in this locale; semantic identifier: $canonical."
            } else {
                "Same visible value as $canonical; kept separate for this property or translation context."
            }
            $null = $entry.AppendChild($comment)
        }
    }

    $settings = [System.Xml.XmlWriterSettings]::new()
    $settings.Encoding = [System.Text.UTF8Encoding]::new($false)
    $settings.Indent = $false
    $writer = [System.Xml.XmlWriter]::Create($path, $settings)
    try {
        $document.Save($writer)
    } finally {
        $writer.Dispose()
    }
}
