# minecraft-map

`client.jar` やリソースパック等のアセットからカラーパレットを自動生成し、Minecraft の地図風の 2D ワールドマップを描画するスタンドアロン型ソフトウェアです。  
A standalone tool that auto-generates a color palette from assets such as `client.jar` and resource packs, then renders a 2D world map in the style of a Minecraft map.

## Example image

Minecraft Vanilla v1.21.8 で自然生成されたワールドデータの出力例です。  
Example output generated from naturally generated world data on Minecraft Vanilla v1.21.8.

- リージョンファイル数 / Region files: 104 (X[-7..4], Z[-5..4])
- 画像サイズ / Image size: 6144x5120 px (1 block = 1 px)
- 生成時間 / Generation time: 42 seconds
- 青紫色 = 未読み込みチャンク、赤紫色 = 未生成チャンク  
  Blue-purple = unloaded chunks, red-purple = ungenerated chunks

![](./example_map.png)

## Development Environment

テスト済み / Tested with:

- Minecraft Java Edition: 1.10.2, 1.21.8, 1.21.11, 26.1
- Go: 1.26.4

多分動作する / Should also work with:

- Minecraft Java Edition 1.5+ (フルなブロックステート対応のため v1.13+ 推奨 / v1.13+ recommended for full block-state support)
- Go 1.21+

## Getting Started

### 1. コンフィグファイルの作成 / Create a config file

`config_example.json` をコピーして `config.json` にリネームし、内容を書き換えます。  
Copy `config_example.json` to `config.json` and edit it as shown below.

```jsonc
{
  // ワールドのリージョンファイル(.mca)のディレクトリ / Directory containing the world's region files (.mca)
  "regionDir": "./world/region",

  // 色の元になるアセット(.jar / .zip / 展開済みディレクトリ)。後に書いたものほど優先される
  // Sources for colors (.jar / .zip / extracted directory). Later entries take priority.
  "resources": [
    ".minecraft/version/26.1.1.jar",
    ".minecraft/resourcepacks/example.zip"
  ],

  // カラーパレットの保存ファイル名 / Color palette file name
  "mapColorJson": "./map_color.json",

  "export": {
    "dir": "./export",   // 画像の出力先フォルダ / Folder to save output images
    "shading": true,     // 高さの違いに陰影をつけるか / Add shading based on height differences
    "byRegion": false,   // リージョンファイルごとに画像を分けるか / Save one image per region file
    "mode": "default"    // PNGの出力方式: "default"(高画質) か "compression"(軽量) / PNG mode: "default" (high quality) or "compression" (smaller file)
  },

  // 各種フォールバック描画色 [R,G,B,A] / Fallback rendering colors [R,G,B,A]
  "fallbackColor": {
    "ungenerated": [106, 90, 205, 255], // まだ生成されていない場所の色 / color for not-yet-generated chunks
    "regionError": [178, 34, 34, 255],  // リージョンファイルが読めなかった場合の色 / color when a region file fails to read
    "chunkError": [255, 0, 0, 255],     // チャンクが読めなかった場合の色 / color when a chunk fails to read
    "parseError": [255, 140, 0, 255],   // チャンクのデータが壊れていた場合の色 / color when chunk data is corrupted
    "void": [0, 0, 0, 0],               // 何もないマス(奈落など)の色。既定は透明 / color for genuinely empty space (e.g. void). Transparent by default
    "missingColor": [255, 255, 0, 255], // 色設定が無いブロックの色 / color for a block with no color defined
    "other": [255, 0, 255, 255]         // その他、想定外の場合の色 / color for any other unexpected case
  },

  // 色が見つからないブロックを、代わりに別のブロックの色で描く設定 / When a block has no color, use another block's color instead
  "fallbackBlocks": {
    "minecraft:snow": "minecraft:snow_height2"
  },

  // マップに描かない(無視する)ブロックのIDリスト / Block IDs to skip when drawing the map
  "suppressBlocks": [
    "minecraft:air",
    "minecraft:cave_air",
    "minecraft:void_air",
    "minecraft:light",
    "minecraft:barrier",
    "minecraft:structure_void"
  ]
}
```

`resources` には次のものを指定できます / You can list the following in `resources`:

- Minecraftの `.jar` ファイル(通常は一番上に書く) / Minecraft's `.jar` file (usually listed first)
- リソースパックの `.zip` ファイル / resource pack `.zip` files
- 展開済みのリソースパック・MODフォルダ / extracted resource pack or mod folders

MOD等の追加コンテンツも、`minecraft`以外の名前空間として自動で読み込まれます。  
Content from mods etc. is also loaded automatically, even outside the `minecraft` namespace.

### 2. カラーパレットの生成 / Generate the color palette

```bash
go run . color
```

#### `export.mode` について / About `export.mode`

| モード / Mode | 内容 / Description |
| --- | --- |
| `default` | 可逆RGBA PNGを出力。色劣化・透過欠落なし。ファイルサイズは大きい。<br>Lossless RGBA PNG. No color loss or transparency loss. Larger file size. |
| `compression` | ImageMagick の `-quantize YUV +dither -colors 256 -type Palette` 相当の256色パレットPNG(color-type 3)を出力。ファイルサイズは大幅に削減されるが、以下の制約あり。<br>Palette PNG (≤256 colors, color-type 3), equivalent to ImageMagick's `-quantize YUV +dither -colors 256 -type Palette`. Much smaller files, with the trade-offs below. |

`compression` モードの制約 / Trade-offs of `compression` mode:

- **色味が変化します(不可逆)** / **Color shift (lossy)**: 出現頻度上位256色のみパレットに残り、他は最近傍色に置き換わります。`shading` 有効時は影のグラデーションでバンディングが出ることがあります。  
  Only the 256 most frequent colors are kept; everything else is replaced by its nearest match. With `shading` enabled, banding can appear in shadow gradients.
- **透過は保持されますが階調は粗くなります** / **Transparency is preserved, but gradation is coarser**: 完全透明(alpha=0)ピクセルが1つでもあれば、必ずパレットに透明枠が1つ確保されます。ただし半透明(水・ガラス等)の濃淡差は限られた枠を奪い合うため、`default` モードより階調が粗くなります。  
  If the image contains at least one fully transparent (alpha=0) pixel, a transparent palette slot is always reserved. However, semi-transparent shades (water, glass, etc.) share a limited number of remaining slots, so gradation is coarser than in `default` mode.

色の正確性・透過を優先するなら `default`、ファイルサイズを優先するなら `compression` を選んでください。  
Choose `default` for color/transparency accuracy, `compression` for smaller file size.

### 3. ワールドマップの出力 / Generate the world map

```bash
go run . generate
```

実行ログに `[WARN] ... missing color blocks ...` が出た場合は、`config.json` の `fallbackBlocks` または `suppressBlocks` に設定を追加してください。  
If you see `[WARN] ... missing color blocks ...` in the log, add an entry to `fallbackBlocks` or `suppressBlocks` in `config.json`.