import { useEffect, useState } from 'react';
import { productAPI } from '../services/api';
import { Link } from 'react-router-dom';

// 处理图片URL，将维基百科页面URL转换为实际图片文件URL
const processImageUrl = (url) => {
  if (!url) return 'https://via.placeholder.com/200';
  
  // 检查是否是维基百科的图片页面URL
  if (url.includes('wikipedia.org/wiki/File:')) {
    // 提取文件名
    const fileName = url.split('/').pop();
    // 构建实际的图片文件URL
    return `https://upload.wikimedia.org/wikipedia/commons/thumb/${fileName.charAt(0)}/${fileName.charAt(0) + fileName.charAt(1)}/${fileName}/200px-${fileName}`;
  }
  
  return url;
};

const Products = () => {
  const [products, setProducts] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchProducts = async () => {
      try {
        const data = await productAPI.listProducts();
        setProducts(data.products || []);
      } catch (error) {
        console.error('Failed to fetch products:', error);
      } finally {
        setLoading(false);
      }
    };

    fetchProducts();
  }, []);

  if (loading) {
    return <div className="loading">加载中...</div>;
  }

  return (
    <div className="products-container">
      <h1>商品列表</h1>
      <div className="products-grid">
        {products.map((product) => (
          <div key={product.id} className="product-card">
            <img 
              src={processImageUrl(product.image_url)} 
              alt={product.name} 
              className="product-image"
            />
            <h3>{product.name}</h3>
            <p className="product-price">¥{product.price}</p>
            <p className="product-stock">库存: {product.stock}</p>
            <Link to={`/products/${product.id}`} className="btn btn-sm">
              查看详情
            </Link>
          </div>
        ))}
      </div>
    </div>
  );
};

export default Products;