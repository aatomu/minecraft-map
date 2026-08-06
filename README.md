# minecraft-map

(Japanase)
`client.jar` のアセットからカラーパレットを自動生成し、Minecraft の地図風の 2D ワールドマップを描画するスタンドアロン型ソフトウェア。

(English)
A standalone software application that automatically generates a color palette from the `client.jar` asset and renders a 2D world map in the style of a Minecraft map.

## Example image

Minecraft Vanilla v1.21.8 で自然生成されたワールドデータで出力したイメージです  
This image was generated using naturally generated world data from Minecraft Vanilla v1.21.8.

- 地図情報 / Map information
  - リージョンファイル数 / Region files: 104 (range: X[-7..4], Z[-5..4])
  - 画像サイズ / Image size: 6144x5120 px (1block=1px)
  - 生成時間 / Generation time: 42 seconds
- 色について / Color information
  - 青紫色: 未読み込みチャンク
  - Blue-purple: unloaded chunks
  - 赤紫色: 未生成チャンク
  - Read-purple: ungenerated chunks

![](./example_map.png)

## Development Environment

Tested with:

- Minecraft Java Edition: 1.21.8, 1.21.11, 26.1
- Go: 1.26.4

Should work on:

- Minecraft Java Edition: 1.13+
- Go: 1.21+

## Getting Started

### 1. コンフィグファイルの作成 / Creating a configuration file

1. `config_example.json`をコピーし、`config.json`にリネームする \
   Copy `config_example.json` and rename it to `config.json`
2. 以下の例に合わせて内容を書き換える \
   Rewrite the content to match the following example

```jsonc
{
  // ワールドのリージョンファイル(.mca) のディレクトリ / Directory containing the world's region file (.mca)
  "regionDir": "./world/region",
  // Minecraftのclient.jarのファイルパス / File path for Minecraft's client.jar
  "clientJar": "~/.minecraft/versions/26.1.1/26.1.1.jar",
  // カラーパレットの保存ファイル名 / Color palette file name
  "mapColorJson": "./map_color.json",
  "export": {
    // 出力先のディレクトリ / Output directory
    "dir": "./export",
    // 高低差による影の描画 / Rendering shadows based on elevation differences
    "shading": true,
    // リージョンファイル(.mca)単位での出力の有効化 /  Enable output by region file (.mca)
    "byRegion": false
  },
  "fallbackColor": {
    // 未生成チャンクのレンダリングカラー / Rendering color for ungenerated chunks
    // [R,G,B,A]
    "ungenerated": [106, 90, 205, 255],
    // 読み取りエラーチャンクのレンダリングカラー / Rendering color for chunks with read errors
    // [R,G,B,A]
    "readError": [255, 0, 0, 255],
    // その他不明なエラー時のレンダリングカラー / Rendering color for other undentified errors
    // [R,G,B,A]
    "other": [255, 0, 255, 255]
  },
  // ブロックカラーが見つからない際のフォールバック設定 / Fallback settings when block colors cannot be found
  // フォールバックは `mapColorJson` を参照する / The fallback refers to `mapColorJson`
  "fallbackBlocks": {
    // "blockID": "fallback blockID"
    "snow": "snow_height2"
  },
  // マップ表示の除外設定にするブロックIDのリスト / List of block IDs to exclude from map display
  "suppressBlocks": ["air", "cave_air", "void_air", "light", "barrier", "structure_void"]
}
```

### 2. カラーパレットの生成 / Generate color palette

以下のコマンドを実行する  
Run the following command

```bash
go run . color
```

### 3. ワールドマップの出力 / Generate world map

以下のコマンドを実行する
Run the following command

```bash
go run . generate
```

実行時のログに出る、`[WARN] ... missing color blocks ...`については、config.json の`fallbackBlocks` もしくは `suppressBlocks` に設定を追加すること  
Regarding the `[WARN] ... missing color blocks ...` message that appears in the runtime log, add a setting to either `fallbackBlocks` or `suppressBlocks` in `config.json`.
