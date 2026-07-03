/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export', // static HTML -> out/ (Golang serve qiladi)
  images: { unoptimized: true },
};

export default nextConfig;
