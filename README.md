# Xray Protocol Panel — Xray-core inbound management

Panel web ringan berbasis Go untuk mengelola instance Xray di VPS Ubuntu ARM64/amd64 dengan NAT provider. Panel mempertahankan inbound VLESS + TCP + REALITY + Vision utama dan dapat menambahkan satu inbound VLESS + XHTTP + REALITY pada listener serta key REALITY terpisah.

## Isi repo

```text
.
├── install.sh                 # install, update, uninstall
├── panel.go                   # server HTTP, autentikasi, API, state
├── config.go                  # validasi, generator config/link, protokol inbound, key
├── config_test.go             # unit test generator dan validasi
├── tests/install_prompts_test.sh # test prompt port internal installer
├── panel_test.go              # test izin file state
├── web.go                     # embed UI ke binary
├── web/index.html             # UI responsif, tanpa CDN
├── cmd/helper/main.go         # helper root dengan aksi yang dibatasi
├── cmd/helper/main_test.go    # test config mode 0600
├── systemd/xray-panel.service # service panel non-root
├── sudoers/xray-panel         # izin helper root tanpa argumen
├── go.mod, go.sum
├── .gitignore                 # mengabaikan binary dan data rahasia
└── vendor/                    # dependency Go vendored untuk build offline dari module proxy
```

## Instalasi satu baris

Jalankan sebagai `root` di Ubuntu 22.04 atau lebih baru. Bentuk yang diminta:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/GeraldZovka/xray-protocol-panel/main/install.sh)
```

Ganti `GeraldZovka/xray-protocol-panel` pada URL raw dengan repo Anda; installer lalu meminta URL clone HTTPS repo untuk menyimpannya ke `/opt/xray-panel`. Agar tak perlu prompt itu, jalankan dengan `XRAY_PANEL_REPO=https://github.com/GeraldZovka/xray-protocol-panel.git` sebelum `bash`.

Pada instalasi baru, installer hanya meminta dua port internal secara manual: port internal panel dan port internal Xray. **Tidak ada nilai bawaan; input kosong atau tidak valid akan diminta ulang.** Kedua port internal harus berbeda. Installer tidak menanyakan hostname atau port publik, jumlah endpoint, maupun daftar mapping. Provider menetapkan hostname dan port publik acak untuk mapping panel dan Xray. Setelah detail mapping tersedia dari provider, arahkan mapping panel ke port internal panel dan tambahkan endpoint Xray di **Pengaturan** panel. Batas provider tetap **9 mapping total**: satu mapping panel dan maksimal **8 endpoint Xray**.

Untuk mapping panel, gunakan hostname:port publik yang ditetapkan provider saat membuka panel, lalu arahkan mapping TCP itu ke port internal panel yang dipilih saat instalasi. Jangan masukkan mapping panel ke daftar endpoint Xray. Setelah provider memberikan detail endpoint Xray, masukkan tiap pasangan `hostname:port` pada kartu inbound terkait di **Pengaturan** panel; setiap mapping TCP diarahkan ke port internal inbound yang bersangkutan. Detail port publik belum tersedia saat instalasi dan nilainya dapat acak, jadi jangan menebak atau menganggap port publik sama dengan port internal. Installer mengecek kedua port internal dan berhenti jika salah satunya sudah dipakai.
Nilai awal non-port tetap: REALITY SNI/dest `www.cloudflare.com` / `www.cloudflare.com:443` dan koneksi VLESS + TCP + REALITY + Vision. Hostname dan port publik endpoint Xray diberikan provider dan diisi melalui panel.

Jika password admin dikosongkan, installer membuat password acak dan menampilkannya **sekali setelah state tersimpan**. Simpan di password manager. Rerun installer tidak mereset password, key, user, atau pengaturan yang tersimpan.

Installer memasang dependency OS dari repositori Ubuntu, meng-clone repo, membangun Go binary dengan satu worker, memasang Xray dari [installer resmi XTLS](https://github.com/XTLS/Xray-install), menyimpan backup config lama, dan memasang unit systemd. Source module Go sudah berada di `vendor/`; build tidak perlu meminta dependency ke Go module proxy.

## NAT, port, dan transport

Kedua profil berjalan sebagai inbound Xray independen dengan port internal TCP masing-masing. Inbound utama VLESS + TCP + REALITY + Vision tetap memakai listener dan key/UUID yang sudah ada. Inbound kedua adalah VLESS + XHTTP + REALITY, menggunakan listener dan key REALITY tersendiri; konfigurasi ini didukung Xray-core untuk transport RAW/TCP dan XHTTP menurut [dokumentasi REALITY resmi](https://xtls.github.io/en/config/transports/reality.html) dan [matriks kompatibilitas transport](https://xtls.github.io/en/config/transport.html). Pengguna yang sama mendapat tautan klien terpisah untuk memilih transport.

Provider menetapkan hostname dan port publik sendiri. Panel tidak menebak atau menambahkan endpoint untuk inbound kedua: setelah provider memberi detailnya, masukkan `hostname:port` pada kartu inbound XHTTP. Arahkan mapping publik sebagai **TCP → port internal inbound terkait**. Mapping panel tetap terpisah; batasnya satu mapping panel dan maksimal **8 endpoint Xray unik total** untuk kedua inbound. Nilai port publik dapat berbeda dari port internal.

Panel menguji seluruh config gabungan menggunakan `xray run -test -config` sebelum penerapan. Jika pengujian atau restart gagal, helper memulihkan config lama. Mengubah mapping di panel tidak mengubah provider NAT secara otomatis; samakan tujuan lokalnya sendiri di UI provider.

Routing yang sudah ada tetap mengirim UDP DNS tujuan port 53 ke outbound `direct` dan memblokir UDP lainnya. Kedua listener VLESS ini memakai TCP, sehingga tidak memerlukan mapping UDP.

## Update dan uninstall

Perbarui panel dan Xray, pertahankan state/password/key/user, lalu validasi config sebelum restart:

```bash
XRAY_PANEL_REPO=https://github.com/GeraldZovka/xray-protocol-panel.git bash <(curl -fsSL https://raw.githubusercontent.com/GeraldZovka/xray-protocol-panel/main/install.sh) update
```

Tombol **Perbarui Xray** di panel hanya menjalankan updater Xray resmi. Pembaruan dapat memutus sesi proxy aktif sesaat. Jika SSH Anda sendiri melewati Xray, update/restart dapat memutus koneksi SSH.

Uninstall **total** (meminta konfirmasi dengan mengetik `hapus-total`):

```bash
XRAY_PANEL_REPO=https://github.com/GeraldZovka/xray-protocol-panel.git bash <(curl -fsSL https://raw.githubusercontent.com/GeraldZovka/xray-protocol-panel/main/install.sh) uninstall
```

Perintah ini menghapus panel dan instalasi Xray melalui uninstaller resmi XTLS, termasuk config aktif, key REALITY, pengguna, state, sertifikat TLS yang dikelola panel, akun service, clone `/opt/xray-panel`, backup lokal yang dibuat panel, dan hanya aturan UFW yang memiliki komentar `xray-panel`. **Tidak ada state atau backup yang disimpan.** Jangan jalankan bila Anda ingin mempertahankan data atau memakai instalasi Xray yang sudah ada untuk layanan lain. Paket OS umum seperti `curl`, Git, Go, `sudo`, UFW, dan riwayat journald bersama sengaja tetap ada agar layanan lain di VPS tidak terganggu. Uninstaller resmi XTLS perlu diunduh; bila jaringan gagal, pembersihan dihentikan sebelum data panel dihapus.

## Profil VLESS dan tautan klien

Panel mengelola dua profil yang berdiri sendiri: **VLESS + TCP + REALITY + Vision** (profil utama yang telah ada) dan **VLESS + XHTTP + REALITY** (profil tambahan opsional). Menambahkan profil kedua tidak mengubah UUID pengguna atau parameter/kunci REALITY profil utama. Profil XHTTP mendapat key REALITY, short ID, dan path sendiri. Untuk VLESS, flow Vision hanya dipakai pada inbound utama TCP; inbound XHTTP tidak mengaktifkan Vision.

Setiap pengguna memperoleh tautan dan QR VLESS untuk setiap endpoint publik yang sudah berisi hostname dan port. Selama provider endpoint inbound XHTTP belum dimasukkan, panel tidak membuat tautannya. URI klien hanya berisi material publik; private key REALITY tidak dikirim dalam API state, tautan, atau QR. File backup memang menyimpan private key kedua profil dan harus diperlakukan sebagai rahasia.

## Panel dan fitur

- Buka URL panel menggunakan hostname dan port publik yang ditetapkan provider untuk mapping panel, misalnya `http://HOST:PORT-PUBLIK-PROVIDER`.
- Login memakai password admin. Password disimpan sebagai bcrypt; sesi cookie `HttpOnly`, `SameSite=Strict`, token CSRF, pemeriksaan Origin, dan rate limit 5 gagal/15 menit per IP.
- Dashboard menunjukkan status/versi/uptime Xray, estimasi CPU host (sampel singkat), load average host, perkiraan RSS proses Xray, kontrol service, dan log terakhir.
- Pengguna dapat ditambah/nonaktifkan/aktifkan/dihapus; UUID VLESS yang sama berlaku pada kedua profil. Link/QR terpisah tersedia hanya untuk endpoint publik lengkap.
- Inbound utama VLESS TCP Vision dipertahankan. Inbound kedua hanya menyediakan VLESS XHTTP REALITY pada port internal lain dan dapat berjalan bersamaan.
- Maksimal 8 endpoint Xray digabung untuk semua inbound (ditambah 1 slot provider panel). Endpoint tiap inbound diperiksa server-side. Xray menjalankan `xray run -test -config` pada seluruh config gabungan sebelum apply; jika uji atau restart gagal, helper root memulihkan config sebelumnya.
- Backup adalah file JSON yang mengandung key REALITY privat untuk tiap inbound, UUID pengguna, dan konfigurasi untuk pemulihan. File tetap rahasia: simpan terenkripsi dan jangan unggah ke repo.
- Dukungan HTTPS: installer dapat membuat sertifikat self-signed bernama `xray-panel.local`, atau memakai cert/key yang Anda sediakan. Sertifikat self-signed tidak cocok dengan hostname provider dan browser akan menampilkan peringatan; untuk HTTPS tepercaya, sediakan sertifikat yang cocok dengan hostname panel atau gunakan reverse proxy TLS. Tanpa TLS, panel menampilkan peringatan HTTP.

### Pengaturan TLS setelah install

Aplikasi menerima `PANEL_TLS_CERT` dan `PANEL_TLS_KEY` melalui `/etc/xray-panel.env`. Jika disetel, file cert/key harus dapat dibaca user `xray-panel` (installer mengaturnya saat setup), lalu:

```bash
sudo systemctl restart xray-panel
```

Untuk reverse proxy HTTPS, bind panel ke loopback dengan mengubah `PANEL_ADDR=127.0.0.1:8080` pada `/etc/xray-panel.env` dan restart service. Bila reverse proxy berjalan lokal, panel mengenali `X-Forwarded-Proto: https` hanya dari koneksi loopback.

## Keamanan dan pilihan desain

- Proses panel berjalan sebagai user Linux non-root `xray-panel`. Satu helper root yang root-owned hanya menerima aksi JSON tetap: apply config, status/log, start/stop/restart Xray, dan update resmi. Sudoers tidak memberi shell atau perintah bebas; helper juga menolak semua argumen command line.
- Admin panel berwenang mengelola proxy dan meminta update Xray. Password minimal 8 byte (gunakan password unik yang lebih panjang) dan batasi akses jaringan ke port panel. Jangan expose panel HTTP ke Internet tanpa HTTPS.
- State panel (`/var/lib/xray-panel/state.json`), config Xray aktif, dan backup config diberi mode `0600`; direktori data panel `0700`. Config aktif ber-owner `root:root`; drop-in systemd memakai `LoadCredential` untuk menyerahkan salinan read-only hanya kepada service Xray non-root saat start, sehingga user `xray` tidak dapat membaca file sumber secara langsung. Ini memerlukan systemd 249 atau lebih baru (Ubuntu 22.04+).
- Private key REALITY disimpan di state/config, tidak dikembalikan pada API biasa, link, QR, atau log UI. Backup yang diunduh memang berisi private key dan harus diperlakukan sebagai rahasia.
- Tidak ada analytics, telemetry, CDN, atau API eksternal di runtime panel. Akses jaringan installer mencakup repo GitHub, GitHub untuk Xray, dan mirror Ubuntu untuk paket OS; permintaan update Xray dari helper mengunduh installer resmi GitHub. Xray sendiri tentu membuat koneksi keluar untuk meneruskan trafik TCP klien proxy.
- Kuota/kedaluwarsa dan statistik trafik per-user belum disertakan. Xray Stats API tidak diaktifkan agar jejak pemakaian dan kompleksitas tetap minimal.

## Checklist pengujian manual

Lakukan lebih dulu pada VPS uji atau dengan backup yang dapat dipulihkan.

1. **Prompt port:** jalankan tes non-destruktif `bash tests/install_prompts_test.sh`. Tes memastikan installer hanya meminta dua port internal, input kosong/invalid diminta ulang, kedua port internal harus berbeda, dan installer tidak menanyakan port publik, jumlah endpoint, atau daftar mapping. Pastikan Pengaturan panel menerima maksimal 8 endpoint Xray (ditambah satu mapping panel, 9 total).
2. **Konflik port:** sebelum instal, pilih port internal yang sudah listen (misalnya `22` jika SSH menggunakannya), masukkan sebagai port internal panel, lalu jalankan installer. Pastikan installer menampilkan proses/port yang bentrok dan berhenti sebelum mengganti config.
3. **Instalasi baru:** pastikan installer selesai, `systemctl status xray xray-panel`, dan `ss -ltnp` menampilkan port internal VLESS utama. Inbound XHTTP kedua bersifat opsional sampai diaktifkan dari panel.
4. **NAT dan firewall:** konfirmasi setiap mapping TCP publik→port internal inbound yang tepat dan tes koneksi TCP. Akses panel melalui URL panel yang berlaku; uji juga dari jaringan berbeda jika memungkinkan.
5. **Login:** password benar masuk; salah 5 kali dibatasi; cookie panel memiliki `HttpOnly`/`SameSite=Strict`. Jika HTTP, peringatan keamanan terlihat.
6. **Pengguna dan QR:** tambah user, salin tautan profil, pindai QR VLESS; pastikan private key tidak ada di link, QR, atau log. Pada staging, aktifkan inbound VLESS XHTTP di port yang berbeda; pastikan config memuat dua inbound, flow Vision hanya pada TCP, key REALITY berbeda, serta link XHTTP mengarah ke endpoint yang dimasukkan. Tambahkan endpoint provider sebelum uji dari luar.
7. **Nonaktif/hapus user:** nonaktifkan user dan pastikan koneksi barunya gagal, aktifkan kembali lalu uji; hapus user dan pastikan hilang dari daftar/config.
8. **Perubahan gagal:** uji path XHTTP yang tidak valid, simpan, pastikan UI menunjukkan error dan konfigurasi sebelumnya tetap bekerja. Untuk uji rollback restart, lakukan hanya di staging dengan port internal yang sengaja bentrok.
9. **Profil XHTTP:** aktifkan inbound kedua, masukkan endpoint yang benar-benar ditetapkan provider, lalu gunakan link XHTTP dari panel pada klien yang mendukung VLESS XHTTP REALITY. Inbound utama tetap TCP Vision.
10. **Backup/restore:** unduh backup, simpan secara aman, ubah user/settings, restore backup dan pastikan config aktif dipulihkan. Pastikan restore tidak mengganti password admin yang sedang dipakai.
11. **Password/TLS:** ubah password admin, pastikan sesi lama harus login ulang; bila TLS diaktifkan, buka `https://HOST:PORT` dan verifikasi bahwa browser menunjukkan TLS (sertifikat self-signed akan tetap diberi warning).
12. **Update/uninstall:** pada staging, uji update dan pastikan pengguna tetap ada. Untuk uninstall total, pastikan konfirmasi `hapus-total` menjelaskan bahwa Xray, konfigurasi, key, pengguna, sertifikat, backup, dan aturan firewall bertanda aplikasi akan dihapus.

## Troubleshooting

- **Panel tidak dapat dibuka:** periksa `systemctl status xray-panel`, `journalctl -u xray-panel -n 100 --no-pager`, `ss -ltnp`, UFW, lalu pastikan mapping publik panel mengarah ke port internal yang benar.
- **Xray gagal restart / port conflict:** periksa `systemctl status xray`, `journalctl -u xray -n 100 --no-pager`, dan `ss -ltnp`. Perubahan melalui panel mengembalikan config sebelumnya saat restart gagal. Periksa pesan error yang ditampilkan UI.
- **Tidak ada koneksi klien:** pastikan hostname dan port publik yang ditetapkan provider sudah dimasukkan sebagai endpoint di Pengaturan dan benar pada link, mapping provider TCP aktif menuju port internal Xray, UFW sesuai, UUID/REALITY public key/short ID benar, dan SNI/dest mendukung TLS 1.3. Pastikan klien memakai transport yang sedang dipilih.
- **Update gagal:** pastikan VPS dapat mengakses GitHub dan mirror Ubuntu, ruang disk cukup, lalu jalankan `install.sh update` lagi setelah memeriksa status Xray. Installer membuat ulang config dari state, menjalankan uji Xray, menyimpan backup, menerapkan perubahan, dan me-rollback bila restart gagal.
- **Lupa password:** tidak ada mekanisme reset dari UI. Setelah login shell root ke VPS, gunakan prosedur reset yang Anda audit sendiri atau backup/restore; jangan mengedit state sembarangan karena memuat key rahasia.
- **Panel tetap HTTP:** file TLS harus pasangan cert/key yang benar, terbaca oleh user `xray-panel`, dan tercantum di `/etc/xray-panel.env`; lihat log service setelah restart.

## Verifikasi yang dilakukan pada paket sumber

Tes prompt installer non-destruktif (`bash tests/install_prompts_test.sh`) dan unit test Go mencakup pasangan VLESS TCP Vision + VLESS XHTTP REALITY, port yang terpisah, key REALITY independen, link XHTTP, batas mapping gabungan, serta pembacaan state legacy. JavaScript UI juga diperiksa syntax-nya. Pada penerapan, helper menguji config lengkap dengan `xray run -test -config` menggunakan versi Xray terpasang sebelum apply dan memulihkan config lama jika gagal. Rincian kombinasi dan batasannya ada di [panduan dua profil VLESS](docs/vless-profiles.md).
