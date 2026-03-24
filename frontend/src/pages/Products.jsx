import { useEffect, useState } from 'react';
import { productAPI } from '../services/api';
import { Link } from 'react-router-dom';

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
              src={product.imageUrl || 'https://via.placeholder.com/200'} 
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