# minecraft-map

(Japanase)
`client.jar` やリソースパック等のアセットからカラーパレットを自動生成し、Minecraft の地図風の 2D ワールドマップを描画するスタンドアロン型ソフトウェア。

(English)
A standalone software application that automatically generates a color palette from assets such as `client.jar` and resource packs, and renders a 2D world map in the style of a Minecraft map.

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

テスト済み / Tested with:

- Minecraft Java Edition: 1.10.2, 1.21.8, 1.21.11, 26.1
- Go: 1.26.4

多分動作する / Should work on:

- Minecraft Java Edition: 1.5+
  (ブロックID仕様の都合上、v1.13+のワールドデータでの利用を推奨 / Recommended v1.13+ for full block-state support)
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
  // アセット読み込み元のリスト(.jar / .zip / 展開済みディレクトリを指定可能) / List of asset sources (.jar / .zip / extracted directory)
  // 配列の先頭が初期値(ベース)、末尾ほど優先度が高く、同じパスのファイルは後方のソースで上書きされる
  // The first entry is the base (default). Later entries take priority and overwrite files at the same path from earlier entries.
  "resources": [
    "~/.minecraft/versions/26.1.1/26.1.1.jar",
    "./resources/some_resourcepack.zip",
    "./resources/my_override_pack"
  ],
  // カラーパレットの保存ファイル名 / Color palette file name
  "mapColorJson": "./map_color.json",
  "export": {
    // 出力先のディレクトリ / Output directory
    "dir": "./export",
    // 高低差による影の描画 / Rendering shadows based on elevation differences
    "shading": true,
    // リージョンファイル(.mca)単位での出力の有効化 /  Enable output by region file (.mca)
    "byRegion": false,
    // PNGファイルの出力モード / PNG file output mode
    // "default" | "compression"
    "mode": "default",
  },
  "fallbackColor": {
    // 未生成チャンクのレンダリングカラー / Rendering color for ungenerated chunks
    // [R,G,B,A]
    "ungenerated": [106, 90, 205, 255],
    // リージョン/チャンクの読み込み(I/O・解凍)失敗時のレンダリングカラー / Rendering color for region/chunk read (I/O, decompress) failures
    // [R,G,B,A]
    "readError": [255, 0, 0, 255],
    // チャンクNBTのパース失敗(データ破損等)時のレンダリングカラー / Rendering color for chunk NBT parse failures (corrupted data, etc.)
    // [R,G,B,A]
    "parseError": [255, 140, 0, 255],
    // 全ブロックがsuppressBlocks対象(air等)だった、正常な空洞(奈落等)のレンダリングカラー
    // 既定は完全透明(alpha=0)にし、他のエラー色と混同しないようにしている
    // Rendering color for a column where every block is a suppressed block (e.g. air) — a genuine void, not an error.
    // Defaults to fully transparent so it's never confused with the error colors above.
    // [R,G,B,A]
    "void": [0, 0, 0, 0],
    // ブロックは検出できたが mapColorJson に該当する色情報が無かった際のレンダリングカラー
    // Rendering color for a block that was detected but has no matching entry in mapColorJson
    // [R,G,B,A]
    "missingColor": [255, 255, 0, 255],
    // 上記いずれにも該当しない、予期しない状態のレンダリングカラー(通常は発生しない)
    // Rendering color for any other unexpected state not covered above (should not normally occur)
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

`resources` に指定できるもの / What can be listed in `resources`:

- `client.jar`（バニラのアセット。基本的に一番最初に指定する / vanilla assets, normally listed first）
- リソースパックの `.zip` ファイル / resource pack `.zip` files
- リソースパックや MOD アセットを展開したディレクトリ / extracted resource pack or mod asset directories

各ソースは `assets/<namespace>/...` を自動的に走査するため、`minecraft` 以外の namespace（MOD 等）も自動で検出されます。  
Each source is automatically scanned under `assets/<namespace>/...`, so namespaces other than `minecraft` (e.g. mods) are detected automatically as well.

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